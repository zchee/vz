// Command custom-virtio boots a Linux virtual machine with a custom Virtio device
// implemented in Go, and logs every CustomVirtioHandler lifecycle callback.
//
// Custom Virtio devices require macOS 27 or newer. Set the kernel, initrd, and
// boot disk via the VMLINUZ_PATH, INITRD_PATH, and DISKIMG_PATH environment
// variables.
//
// This is a host-side demonstration: DidCreateDevice fires at construction and the
// lifecycle callbacks fire on VM state changes, but the virtqueue data plane
// (DidReceiveNotificationForQueue) needs a matching guest driver not present in a
// stock kernel and will not fire here.
package main

import (
	"fmt"
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

// customVirtioDeviceConfiguration builds a custom Virtio device: the discovery
// properties a guest needs to detect it, an optional feature bit, a shared-memory
// region, save/restore support, and a handler that logs every lifecycle callback.
func customVirtioDeviceConfiguration() (*vz.CustomVirtioDeviceConfiguration, error) {
	cvdc, err := vz.NewCustomVirtioDeviceConfiguration()
	if err != nil {
		return nil, err
	}
	cvdc.SetDeviceID(0x1AF4) // an example Virtio device ID
	cvdc.SetPCIClassID(0x02)
	cvdc.SetPCISubclassID(0x00)
	cvdc.SetVirtioQueueCount(1)
	cvdc.OptionalFeatures().SetSubset0(0x1) // an example optional feature bit

	// A 1 MiB shared-memory region (region ID 0) and save/restore support, to
	// exercise the rest of the configuration surface.
	region, err := vz.NewVirtioSharedMemoryRegionConfiguration(0, 1<<20)
	if err != nil {
		return nil, err
	}
	cvdc.SetSharedMemoryRegions([]*vz.VirtioSharedMemoryRegionConfiguration{region})
	cvdc.SetSupportsSaveRestore(true)

	cvdc.SetHandler(vz.CustomVirtioHandler{
		DidCreateDevice: func(*vz.CustomVirtioDevice) { log.Println("custom Virtio: DidCreateDevice") },
		DidAcceptDriverOK: func(*vz.CustomVirtioDevice) {
			log.Println("custom Virtio: DidAcceptDriverOK (guest reached DRIVER_OK)")
		},
		DidReceiveNotificationForQueue: func(_ *vz.CustomVirtioDevice, q *vz.VirtioQueue) {
			log.Printf("custom Virtio: DidReceiveNotificationForQueue (queue %d)", q.QueueIndex())
		},
		WillStop:   func(*vz.CustomVirtioDevice) { log.Println("custom Virtio: WillStop") },
		WillPause:  func(*vz.CustomVirtioDevice) { log.Println("custom Virtio: WillPause") },
		WillResume: func(*vz.CustomVirtioDevice) { log.Println("custom Virtio: WillResume") },
		WillReset:  func(*vz.CustomVirtioDevice) { log.Println("custom Virtio: WillReset") },
		// The device carries no state to save: return an empty (non-nil) slice.
		SaveStateForRestore: func(*vz.CustomVirtioDevice) []byte { return []byte{} },
		ShouldRestore:       func(_ *vz.CustomVirtioDevice, _ []byte) bool { return true },
	})
	return cvdc, nil
}

func main() {
	fmt.Fprintln(os.Stderr, "custom-virtio: booting a Linux VM with a custom Virtio device (requires macOS 27+).")
	fmt.Fprintln(os.Stderr, "DidCreateDevice fires at construction and lifecycle callbacks fire on VM state")
	fmt.Fprintln(os.Stderr, "changes, but the virtqueue data plane (DidReceiveNotificationForQueue) needs a")
	fmt.Fprintln(os.Stderr, "matching guest driver not present in a stock kernel, so it will not fire here.")
	fmt.Fprintln(os.Stderr, "Lifecycle callbacks are logged to ./log.log.")

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

	// custom Virtio device (macOS 27+): the discovery config plus a handler that
	// logs every lifecycle callback. See customVirtioDeviceConfiguration and the
	// startup banner for what fires and what needs a guest driver.
	cvdc, err := customVirtioDeviceConfiguration()
	if err != nil {
		log.Fatalf("custom Virtio device creation failed (requires macOS 27+): %s", err)
	}
	config.SetCustomVirtioDevicesVirtualMachineConfiguration([]*vz.CustomVirtioDeviceConfiguration{
		cvdc,
	})

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
