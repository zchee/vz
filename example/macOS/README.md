Example
=======

You can get knowledge build and codesign process in Makefile.

## Build

```sh
make all
```

## Run

- `./virtualization -install` install macOS to your VM.
- `./virtualization` run macOS VM.
- `./virtualization -provision` on the **first boot after install**, provision the
  guest (create a user account, enable automatic login and Remote Login/SSH).

## Guest provisioning

`-provision` (macOS 27+) applies `VZMacGuestProvisioningOptions` to the first boot
after a restore, so run it once after `-install`. It needs a downloaded macOS
restore image and a full install to complete, so it cannot be exercised in CI — it
is an image-gated manual step.

## Resources

Some resources will be created in `VM.bundle` directory on your home directory.