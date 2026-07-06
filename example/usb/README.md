USB example
===========

Boots a Linux VM with a USB XHCI controller carrying a USB mass-storage device
backed by a disk image, demonstrating `NewXHCIControllerConfiguration`,
`NewUSBMassStorageDeviceConfiguration`, and
`SetUSBControllersVirtualMachineConfiguration`.

The USB controller and USB devices require **macOS 15 or newer**. Unlike USB
passthrough, this example needs no physical hardware — just a spare disk image
for the USB storage backing file.

## Build

```sh
make all
```

`make all` builds the `usb` binary and codesigns it with `vz.entitlements`
(a process must hold the `com.apple.security.virtualization` entitlement to use
the Virtualization APIs).

## Run

Set the kernel/initrd/boot-disk paths (as in the `linux` example) plus a
separate backing image for the USB storage device, then run the signed binary:

```sh
export VMLINUZ_PATH=/path/to/vmlinuz
export INITRD_PATH=/path/to/initrd
export DISKIMG_PATH=/path/to/boot.img
export USB_DISKIMG_PATH=/path/to/usb-storage.img
./usb
```

Create the USB backing image beforehand, for example a 64 MiB blank image:

```sh
dd if=/dev/zero of=/path/to/usb-storage.img bs=1m count=64
```

## Verify in the guest

Once the VM has booted, the guest sees the USB mass-storage device. Confirm it
from inside the guest:

```sh
lsblk
dmesg | grep -i usb
```
