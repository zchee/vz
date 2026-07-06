USB passthrough (macOS 27+)
===========================

This is **reference documentation, not a runnable example.** USB passthrough hands
a physical USB accessory attached to the host through to the guest. It cannot be
demonstrated by a plain `go run` CLI — see the requirements below.

## Why this is not a runnable CLI example

The vz binding wraps Apple's AccessoryAccess framework to obtain the accessory. Its
constraints (resolved from the macOS 27.0 SDK headers and documented verbatim in
`accessory_access.go`) are:

- **Entitlement + Dock app.** From `accessory_access.go`:
  > an app using AAUSBAccessoryManager must be signed with the
  > "com.apple.developer.accessory-access.usb" entitlement, and must be an
  > ordinary (Dock) application, because the manager presents consent UI on
  > the application's behalf. FindUSBAccessories therefore cannot run in a
  > headless/entitlement-less context.
- **IOKit matching dictionary.** From `accessory_access.go`:
  > AAUSBAccessoryMatchingCriteria requires a real IOKit USB matching dictionary
  > produced by
  > +[IOUSBHostDevice createMatchingDictionaryWithVendorID:productID:...] — a
  > hand-built dictionary of USB property keys is rejected (initWith… returns
  > nil). So the wrapper builds it from vendor/product IDs and loads
  > IOUSBHost.framework in addition to AccessoryAccess.
- **The framework captures the device, not the app.** From `accessory_access.go`:
  > the app does NOT open the AAUSBAccessory. The Virtualization framework
  > captures the device itself when the VM starts with the passthrough
  > configuration, or when -[VZUSBController attachDevice:] runs; so this wrapper
  > deliberately omits open/close/XPC.

So a working passthrough setup needs: a **physical USB accessory**, a binary signed
with the **`com.apple.developer.accessory-access.usb`** entitlement (in addition to
`com.apple.security.virtualization`), running as an **ordinary Dock application**
that can present the **user-consent** prompt AccessoryAccess raises. None of that is
possible from a headless `go run` on the command line, which is why this is a doc
rather than a program.

## The flow

Discover the accessory by vendor/product ID, build a passthrough device
configuration from it, and attach it to an XHCI controller — the same controller
used in the runnable `example/usb` (`main.go`):

```go
// Requires macOS 27+, the com.apple.developer.accessory-access.usb entitlement,
// and an ordinary Dock application (not a headless CLI).
criteria, err := vz.NewUSBAccessoryMatchingCriteria(0x1234, 0x5678) // vendorID, productID
if err != nil {
	return err
}

// Presents a user-consent prompt; returns the matching accessories.
accessories, err := vz.FindUSBAccessories(criteria)
if err != nil {
	return err
}
if len(accessories) == 0 {
	return fmt.Errorf("no matching USB accessory found")
}
accessory := accessories[0] // *vz.USBAccessory; see RegistryID/DeviceDescriptor/ConfigurationDescriptor

passthroughCfg, err := vz.NewUSBPassthroughDeviceConfiguration(accessory)
if err != nil {
	return err
}

// Attach it to an XHCI controller, exactly like example/usb attaches the mass-
// storage device (a *USBPassthroughDeviceConfiguration is a vz.USBDeviceConfiguration).
xhci, err := vz.NewXHCIControllerConfiguration()
if err != nil {
	return err
}
xhci.SetUSBDevices([]vz.USBDeviceConfiguration{passthroughCfg})
config.SetUSBControllersVirtualMachineConfiguration([]vz.USBControllerConfiguration{xhci})
```

The framework captures the accessory when the VM starts. For runtime hot-plug after
start, build a runtime device with `vz.NewUSBPassthroughDevice(passthroughCfg)` and
`controller.Attach(device)` on the XHCI controller obtained from
`vm.USBControllers()`.

See the runnable `example/usb` for the surrounding VM setup and the signing flow.
