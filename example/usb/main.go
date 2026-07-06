// Command usb boots a Linux virtual machine with a USB XHCI controller carrying
// a USB mass-storage device backed by a disk image. It mirrors the linux example
// and adds the USB controller so a guest enumerates a USB storage device.
//
// The USB controller and USB devices require macOS 15 or newer. Set the kernel,
// initrd, boot disk, and a separate USB backing image via the VMLINUZ_PATH,
// INITRD_PATH, DISKIMG_PATH, and USB_DISKIMG_PATH environment variables.
package main

import (
	l "log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Code-Hex/vz/v3"
	"github.com/pkg/term/termios"
	"golang.org/x/sys/unix"
)

var log *l.Logger

// https://developer.apple.com/documentation/virtualization/running_linux_in_a_virtual_machine?language=objc#:~:text=Configure%20the%20Serial%20Port%20Device%20for%20Standard%20In%20and%20Out
func setRawMode(f *os.File) {
	var attr unix.Termios

	// Get settings for terminal
	termios.Tcgetattr(f.Fd(), &attr)

	// Put stdin into raw mode, disabling local echo, input canonicalization,
	// and CR-NL mapping.
	attr.Iflag &^= syscall.ICRNL
	attr.Lflag &^= syscall.ICANON | syscall.ECHO

	// Set minimum characters when reading = 1 char
	attr.Cc[syscall.VMIN] = 1

	// set timeout when reading as non-canonical mode
	attr.Cc[syscall.VTIME] = 0

	// reflects the changed settings
	termios.Tcsetattr(f.Fd(), termios.TCSANOW, &attr)
}

func main() {
	file, err := os.Create("./log.log")
	if err != nil {
		panic(err)
	}
	defer file.Close()
	log = l.New(file, "", l.LstdFlags)

	kernelCommandLineArguments := []string{
		// Use the first virtio console device as system console.
		"console=hvc0",
		// Stop in the initial ramdisk before attempting to transition to
		// the root file system.
		"root=/dev/vda",
	}

	vmlinuz := os.Getenv("VMLINUZ_PATH")
	initrd := os.Getenv("INITRD_PATH")
	diskPath := os.Getenv("DISKIMG_PATH")
	usbDiskPath := os.Getenv("USB_DISKIMG_PATH")

	bootLoader, err := vz.NewLinuxBootLoader(
		vmlinuz,
		vz.WithCommandLine(strings.Join(kernelCommandLineArguments, " ")),
		vz.WithInitrd(initrd),
	)
	if err != nil {
		log.Fatalf("bootloader creation failed: %s", err)
	}
	log.Println("bootLoader:", bootLoader)

	config, err := vz.NewVirtualMachineConfiguration(
		bootLoader,
		1,
		2*1024*1024*1024,
	)
	if err != nil {
		log.Fatalf("failed to create virtual machine configuration: %s", err)
	}

	setRawMode(os.Stdin)

	// console
	serialPortAttachment, err := vz.NewFileHandleSerialPortAttachment(os.Stdin, os.Stdout)
	if err != nil {
		log.Fatalf("Serial port attachment creation failed: %s", err)
	}
	consoleConfig, err := vz.NewVirtioConsoleDeviceSerialPortConfiguration(serialPortAttachment)
	if err != nil {
		log.Fatalf("Failed to create serial configuration: %s", err)
	}
	config.SetSerialPortsVirtualMachineConfiguration([]*vz.VirtioConsoleDeviceSerialPortConfiguration{
		consoleConfig,
	})

	// network
	natAttachment, err := vz.NewNATNetworkDeviceAttachment()
	if err != nil {
		log.Fatalf("NAT network device creation failed: %s", err)
	}
	networkConfig, err := vz.NewVirtioNetworkDeviceConfiguration(natAttachment)
	if err != nil {
		log.Fatalf("Creation of the networking configuration failed: %s", err)
	}
	config.SetNetworkDevicesVirtualMachineConfiguration([]*vz.VirtioNetworkDeviceConfiguration{
		networkConfig,
	})
	mac, err := vz.NewRandomLocallyAdministeredMACAddress()
	if err != nil {
		log.Fatalf("Random MAC address creation failed: %s", err)
	}
	networkConfig.SetMACAddress(mac)

	// entropy
	entropyConfig, err := vz.NewVirtioEntropyDeviceConfiguration()
	if err != nil {
		log.Fatalf("Entropy device creation failed: %s", err)
	}
	config.SetEntropyDevicesVirtualMachineConfiguration([]*vz.VirtioEntropyDeviceConfiguration{
		entropyConfig,
	})

	// boot disk (virtio-block)
	diskImageAttachment, err := vz.NewDiskImageStorageDeviceAttachment(
		diskPath,
		false,
	)
	if err != nil {
		log.Fatal(err)
	}
	storageDeviceConfig, err := vz.NewVirtioBlockDeviceConfiguration(diskImageAttachment)
	if err != nil {
		log.Fatalf("Block device creation failed: %s", err)
	}
	config.SetStorageDevicesVirtualMachineConfiguration([]vz.StorageDeviceConfiguration{
		storageDeviceConfig,
	})

	// USB: an XHCI controller carrying a USB mass-storage device backed by a
	// separate disk image. Requires macOS 15 or newer. After boot, the guest
	// enumerates the device; verify it with `lsblk` or `dmesg | grep -i usb`.
	usbAttachment, err := vz.NewDiskImageStorageDeviceAttachment(usbDiskPath, false)
	if err != nil {
		log.Fatalf("USB disk image attachment creation failed: %s", err)
	}
	usbMassStorage, err := vz.NewUSBMassStorageDeviceConfiguration(usbAttachment)
	if err != nil {
		log.Fatalf("USB mass storage device creation failed: %s", err)
	}
	xhciController, err := vz.NewXHCIControllerConfiguration()
	if err != nil {
		log.Fatalf("XHCI controller creation failed (requires macOS 15+): %s", err)
	}
	xhciController.SetUSBDevices([]vz.USBDeviceConfiguration{usbMassStorage})
	config.SetUSBControllersVirtualMachineConfiguration([]vz.USBControllerConfiguration{
		xhciController,
	})
	log.Println("attached a USB XHCI controller with a USB mass-storage device; verify in the guest with `lsblk` / `dmesg | grep -i usb`")

	// traditional memory balloon device which allows for managing guest memory. (optional)
	memoryBalloonDevice, err := vz.NewVirtioTraditionalMemoryBalloonDeviceConfiguration()
	if err != nil {
		log.Fatalf("Balloon device creation failed: %s", err)
	}
	config.SetMemoryBalloonDevicesVirtualMachineConfiguration([]vz.MemoryBalloonDeviceConfiguration{
		memoryBalloonDevice,
	})

	// socket device (optional)
	vsockDevice, err := vz.NewVirtioSocketDeviceConfiguration()
	if err != nil {
		log.Fatalf("virtio-vsock device creation failed: %s", err)
	}
	config.SetSocketDevicesVirtualMachineConfiguration([]vz.SocketDeviceConfiguration{
		vsockDevice,
	})

	validated, err := config.Validate()
	if !validated || err != nil {
		log.Fatal("validation failed", err)
	}

	vm, err := vz.NewVirtualMachine(config)
	if err != nil {
		log.Fatalf("Virtual machine creation failed: %s", err)
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGTERM)

	if err := vm.Start(); err != nil {
		log.Fatalf("Start virtual machine is failed: %s", err)
	}

	errCh := make(chan error, 1)

	for {
		select {
		case <-signalCh:
			result, err := vm.RequestStop()
			if err != nil {
				log.Println("request stop error:", err)
				return
			}
			log.Println("recieved signal", result)
		case newState := <-vm.StateChangedNotify():
			if newState == vz.VirtualMachineStateRunning {
				log.Println("start VM is running")
			}
			if newState == vz.VirtualMachineStateStopped {
				log.Println("stopped successfully")
				return
			}
		case err := <-errCh:
			log.Println("in start:", err)
		}
	}
}
