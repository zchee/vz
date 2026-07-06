Custom Virtio example
=====================

Boots a Linux VM with a **custom Virtio device** implemented in Go, demonstrating
`NewCustomVirtioDeviceConfiguration`, the `CustomVirtioHandler` lifecycle
callbacks, a `VirtioSharedMemoryRegion`, save/restore support, and
`SetCustomVirtioDevicesVirtualMachineConfiguration`.

Custom Virtio devices require **macOS 27 or newer**.

## What runs, and what does not

This is a **host-side** demonstration. The framework calls `DidCreateDevice` when
it builds the device (at VM construction), and calls the lifecycle callbacks
(`WillStop` / `WillPause` / `WillResume` / `WillReset`) on the VM's state changes.
`DidAcceptDriverOK` fires only if the guest's generic virtio-pci layer probes and
accepts the (unknown) device, which a stock kernel may or may not do.

The **virtqueue data plane** — `DidReceiveNotificationForQueue` — will **not**
fire with a stock kernel: it requires a matching **guest driver** (a Linux kernel
module or userspace virtio driver) that claims this device and kicks a virtqueue.
Writing that guest driver is a separate effort, out of scope for this example.

Every lifecycle callback is logged to `./log.log`; the program also prints a
banner explaining this boundary on startup.

## Build

```sh
make all
```

`make all` builds the `custom-virtio` binary and codesigns it with
`vz.entitlements` (a process must hold the `com.apple.security.virtualization`
entitlement to use the Virtualization APIs).

## Run

Set the kernel/initrd/boot-disk paths (as in the `linux` example) and run the
signed binary:

```sh
export VMLINUZ_PATH=/path/to/vmlinuz
export INITRD_PATH=/path/to/initrd
export DISKIMG_PATH=/path/to/boot.img
./custom-virtio
```

Then read `./log.log` to see which callbacks fired.
