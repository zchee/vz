//go:build darwin && arm64
// +build darwin,arm64

package vz

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
	"github.com/Code-Hex/vz/v3/internal/progress"
)

// vzMacAuxiliaryStorageInitializationOptionAllowOverwrite mirrors
// VZMacAuxiliaryStorageInitializationOptionAllowOverwrite.
const vzMacAuxiliaryStorageInitializationOptionAllowOverwrite = 1 << 0

// WithStartUpFromMacOSRecovery is an option to specifiy whether to start up
// from macOS Recovery for macOS VM.
//
// This is only supported on macOS 13 and newer, error will
// be returned on older versions.
func WithStartUpFromMacOSRecovery(startInRecovery bool) VirtualMachineStartOption {
	return func(vmso *virtualMachineStartOptions) error {
		if err := macOSAvailable(13); err != nil {
			return err
		}
		opts := objc.New("VZMacOSVirtualMachineStartOptions", "init")
		objc.SendVoid(opts, "setStartUpFromMacOSRecovery:", startInRecovery)
		vmso.macOSVirtualMachineStartOptionsPtr = opts
		return nil
	}
}

// MacHardwareModel describes a specific virtual Mac hardware model.
type MacHardwareModel struct {
	*pointer

	supported          bool
	dataRepresentation []byte
}

// NewMacHardwareModelWithDataPath initialize a new hardware model described by the specified pathname.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewMacHardwareModelWithDataPath(pathname string) (*MacHardwareModel, error) {
	b, err := os.ReadFile(pathname)
	if err != nil {
		return nil, err
	}
	return NewMacHardwareModelWithData(b)
}

// NewMacHardwareModelWithData initialize a new hardware model described by the specified data representation.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewMacHardwareModelWithData(b []byte) (*MacHardwareModel, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}

	data := objc.NSData(b)
	defer objc.SendVoid(data, "release")

	ret := newMacHardwareModel(
		objc.New("VZMacHardwareModel", "initWithDataRepresentation:", data),
	)
	objc.SetFinalizer(ret, func(self *MacHardwareModel) {
		objc.Release(self)
	})
	return ret, nil
}

func newMacHardwareModel(ptr unsafe.Pointer) *MacHardwareModel {
	return &MacHardwareModel{
		pointer:            objc.NewPointer(ptr),
		supported:          objc.Send[bool](objc.ID(uintptr(ptr)), objc.RegisterName("isSupported")),
		dataRepresentation: objc.NSDataToBytes(objc.SendPtr(ptr, "dataRepresentation")),
	}
}

// Supported indicate whether this hardware model is supported by the host.
func (m *MacHardwareModel) Supported() bool { return m.supported }

// DataRepresentation opaque data representation of the hardware model.
// This can be used to recreate the same hardware model with NewMacHardwareModelWithData function.
func (m *MacHardwareModel) DataRepresentation() []byte { return m.dataRepresentation }

// MacMachineIdentifier an identifier to make a virtual machine unique.
type MacMachineIdentifier struct {
	*pointer

	dataRepresentation []byte
}

// NewMacMachineIdentifierWithDataPath initialize a new machine identifier described by the specified pathname.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewMacMachineIdentifierWithDataPath(pathname string) (*MacMachineIdentifier, error) {
	b, err := os.ReadFile(pathname)
	if err != nil {
		return nil, err
	}
	return NewMacMachineIdentifierWithData(b)
}

// NewMacMachineIdentifierWithData initialize a new machine identifier described by the specified data representation.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewMacMachineIdentifierWithData(b []byte) (*MacMachineIdentifier, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}

	data := objc.NSData(b)
	defer objc.SendVoid(data, "release")

	return newMacMachineIdentifier(
		objc.New("VZMacMachineIdentifier", "initWithDataRepresentation:", data),
	), nil
}

// NewMacMachineIdentifier initialize a new Mac machine identifier is used by macOS guests to uniquely
// identify the virtual hardware.
//
// Two virtual machines running concurrently should not use the same identifier.
//
// If the virtual machine is serialized to disk, the identifier can be preserved in a binary representation through
// DataRepresentation method.
// The identifier can then be recreated with NewMacMachineIdentifierWithData function from the binary representation.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewMacMachineIdentifier() (*MacMachineIdentifier, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	return newMacMachineIdentifier(
		objc.New("VZMacMachineIdentifier", "init"),
	), nil
}

func newMacMachineIdentifier(ptr unsafe.Pointer) *MacMachineIdentifier {
	return &MacMachineIdentifier{
		pointer:            objc.NewPointer(ptr),
		dataRepresentation: objc.NSDataToBytes(objc.SendPtr(ptr, "dataRepresentation")),
	}
}

// DataRepresentation opaque data representation of the machine identifier.
// This can be used to recreate the same machine identifier with NewMacMachineIdentifierWithData function.
func (m *MacMachineIdentifier) DataRepresentation() []byte { return m.dataRepresentation }

// MacAuxiliaryStorage is a struct that contains information the boot loader
// needs for booting macOS as a guest operating system.
type MacAuxiliaryStorage struct {
	*pointer

	storagePath string
}

// NewMacAuxiliaryStorageOption is an option type to initialize a new Mac auxiliary storage
type NewMacAuxiliaryStorageOption func(*MacAuxiliaryStorage) error

// WithCreatingMacAuxiliaryStorage is an option when initialize a new Mac auxiliary storage with data creation
// to you specified storage path.
func WithCreatingMacAuxiliaryStorage(hardwareModel *MacHardwareModel) NewMacAuxiliaryStorageOption {
	return func(mas *MacAuxiliaryStorage) error {
		errSlot := objc.NewErrorSlot()
		defer objc.Free(errSlot)

		mas.pointer = objc.NewPointer(
			objc.New(
				"VZMacAuxiliaryStorage", "initCreatingStorageAtURL:hardwareModel:options:error:",
				objc.FileURL(mas.storagePath),
				objc.Ptr(hardwareModel),
				uint(vzMacAuxiliaryStorageInitializationOptionAllowOverwrite),
				errSlot,
			),
		)
		if err := newNSError(objc.ErrorFromSlot(errSlot)); err != nil {
			return err
		}
		return nil
	}
}

// NewMacAuxiliaryStorage creates a new MacAuxiliaryStorage is based Mac auxiliary storage data from the storagePath
// of an existing file by default.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewMacAuxiliaryStorage(storagePath string, opts ...NewMacAuxiliaryStorageOption) (*MacAuxiliaryStorage, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}

	storage := &MacAuxiliaryStorage{storagePath: storagePath}
	for _, opt := range opts {
		if err := opt(storage); err != nil {
			return nil, err
		}
	}

	if objc.Ptr(storage) == nil {
		storage.pointer = objc.NewPointer(
			objc.New("VZMacAuxiliaryStorage", "initWithContentsOfURL:", objc.FileURL(storagePath)),
		)
	}
	return storage, nil
}

// MacOSRestoreImage is a struct that describes a version of macOS to install on to a virtual machine.
type MacOSRestoreImage struct {
	url                                     string
	buildVersion                            string
	operatingSystemVersion                  OperatingSystemVersion
	mostFeaturefulSupportedConfigurationPtr unsafe.Pointer
}

// URL returns URL of this restore image.
// the value of this property will be a file URL. (https://~)
// the value of this property will be a network URL referring to an installation media file. (file:///~)
func (m *MacOSRestoreImage) URL() string {
	return m.url
}

// BuildVersion returns the build version this restore image contains.
func (m *MacOSRestoreImage) BuildVersion() string {
	return m.buildVersion
}

// OperatingSystemVersion represents the operating system version this restore image contains.
type OperatingSystemVersion struct {
	MajorVersion int64
	MinorVersion int64
	PatchVersion int64
}

// String returns string for the build version this restore image contains.
func (osv OperatingSystemVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", osv.MajorVersion, osv.MinorVersion, osv.PatchVersion)
}

// OperatingSystemVersion returns the operating system version this restore image contains.
func (m *MacOSRestoreImage) OperatingSystemVersion() OperatingSystemVersion {
	return m.operatingSystemVersion
}

// MostFeaturefulSupportedConfiguration returns the configuration requirements for the most featureful
// configuration supported by the current host and by this restore image.
//
// A MacOSRestoreImage can contain installation media for multiple Mac hardware models (MacHardwareModel). Some of these
// hardware models may not be supported by the current host. This method can be used to determine the hardware model and
// configuration requirements that will provide the most complete feature set on the current host.
// If none of the hardware models are supported on the current host, this property is nil.
func (m *MacOSRestoreImage) MostFeaturefulSupportedConfiguration() *MacOSConfigurationRequirements {
	return newMacOSConfigurationRequirements(m.mostFeaturefulSupportedConfigurationPtr)
}

// nsOperatingSystemVersion mirrors Foundation's NSOperatingSystemVersion struct
// (three NSInteger fields), used for the struct-returning
// -[VZMacOSRestoreImage operatingSystemVersion] message.
type nsOperatingSystemVersion struct {
	Major int64
	Minor int64
	Patch int64
}

// newMacOSRestoreImageFromPtr builds a *MacOSRestoreImage by reading the
// properties of a VZMacOSRestoreImage object. The mostFeaturefulSupportedConfiguration
// pointer is retained because it is kept beyond the lifetime of the framework's
// completion callback.
func newMacOSRestoreImageFromPtr(restoreImagePtr unsafe.Pointer) *MacOSRestoreImage {
	if restoreImagePtr == nil {
		return nil
	}
	id := objc.ID(uintptr(restoreImagePtr))
	urlObj := objc.SendPtr(restoreImagePtr, "URL")
	absStr := objc.SendPtr(urlObj, "absoluteString")
	buildVerObj := objc.SendPtr(restoreImagePtr, "buildVersion")
	osv := objc.Send[nsOperatingSystemVersion](id, objc.RegisterName("operatingSystemVersion"))

	config := objc.SendPtr(restoreImagePtr, "mostFeaturefulSupportedConfiguration")
	if config != nil {
		objc.SendVoid(config, "retain")
	}

	return &MacOSRestoreImage{
		url:          objc.GoString(objc.SendPtr(absStr, "UTF8String")),
		buildVersion: objc.GoString(objc.SendPtr(buildVerObj, "UTF8String")),
		operatingSystemVersion: OperatingSystemVersion{
			MajorVersion: osv.Major,
			MinorVersion: osv.Minor,
			PatchVersion: osv.Patch,
		},
		mostFeaturefulSupportedConfigurationPtr: config,
	}
}

// MacOSConfigurationRequirements describes the parameter constraints required by a specific configuration of macOS.
//
// When a VZMacOSRestoreImage is loaded, it can be inspected to determine the configurations supported by that restore image.
type MacOSConfigurationRequirements struct {
	minimumSupportedCPUCount   uint64
	minimumSupportedMemorySize uint64
	hardwareModelPtr           unsafe.Pointer
}

func newMacOSConfigurationRequirements(ptr unsafe.Pointer) *MacOSConfigurationRequirements {
	id := objc.ID(uintptr(ptr))
	// hardwareModel is retained because it is kept beyond this conversion.
	hardwareModel := objc.SendPtr(ptr, "hardwareModel")
	if hardwareModel != nil {
		objc.SendVoid(hardwareModel, "retain")
	}
	return &MacOSConfigurationRequirements{
		minimumSupportedCPUCount:   objc.Send[uint64](id, objc.RegisterName("minimumSupportedCPUCount")),
		minimumSupportedMemorySize: objc.Send[uint64](id, objc.RegisterName("minimumSupportedMemorySize")),
		hardwareModelPtr:           hardwareModel,
	}
}

// HardwareModel returns the hardware model for this configuration.
//
// The hardware model can be used to configure a new virtual machine that meets the requirements.
// Use VZMacPlatformConfiguration.hardwareModel to configure the Mac platform, and
// Use `WithCreatingStorage` functional option of the `NewMacAuxiliaryStorage` to create its auxiliary storage.
func (m *MacOSConfigurationRequirements) HardwareModel() *MacHardwareModel {
	return newMacHardwareModel(m.hardwareModelPtr)
}

// MinimumSupportedCPUCount returns the minimum supported number of CPUs for this configuration.
func (m *MacOSConfigurationRequirements) MinimumSupportedCPUCount() uint64 {
	return m.minimumSupportedCPUCount
}

// MinimumSupportedMemorySize returns the minimum supported memory size for this configuration.
func (m *MacOSConfigurationRequirements) MinimumSupportedMemorySize() uint64 {
	return m.minimumSupportedMemorySize
}

// downloadRestoreImage resumable downloads macOS restore image (ipsw) file.
func downloadRestoreImage(ctx context.Context, url, destPath string) (*progress.Reader, error) {
	// open or create
	f, err := os.OpenFile(destPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		return nil, err
	}

	fileInfo, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		f.Close()
		return nil, err
	}

	req.Header.Add("User-Agent", "github.com/Code-Hex/vz")
	req.Header.Add("Range", fmt.Sprintf("bytes=%d-", fileInfo.Size()))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		f.Close()
		return nil, err
	}

	if 200 > resp.StatusCode || resp.StatusCode >= 300 {
		f.Close()
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected http status code: %d", resp.StatusCode)
	}

	reader := progress.NewReader(resp.Body, resp.ContentLength, fileInfo.Size())

	go func() {
		defer f.Close()
		defer resp.Body.Close()
		_, err := io.Copy(f, reader)
		reader.Finish(err)
	}()

	return reader, nil
}

// GetLatestSupportedMacOSRestoreImageURL get the latest macOS restore image url supported by this host from the network.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func GetLatestSupportedMacOSRestoreImageURL() (string, error) {
	if err := macOSAvailable(12); err != nil {
		return "", err
	}
	type result struct {
		url string
		err error
	}
	ch := make(chan result, 1)
	block := objc.BlockObjectError(func(imgPtr, errPtr unsafe.Pointer) {
		if err := newNSError(errPtr); err != nil {
			ch <- result{err: err}
			return
		}
		ch <- result{url: newMacOSRestoreImageFromPtr(imgPtr).URL()}
	})
	objc.ID(objc.GetClass("VZMacOSRestoreImage")).Send(
		objc.RegisterName("fetchLatestSupportedWithCompletionHandler:"),
		block,
	)
	res := <-ch
	block.Release()
	if res.err != nil {
		return "", res.err
	}
	return res.url, nil
}

// FetchLatestSupportedMacOSRestoreImage fetches the latest macOS restore image supported by this host from the network.
//
// After downloading the restore image, you can initialize a MacOSInstaller using LoadMacOSRestoreImageFromPath function
// with the local restore image file.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func FetchLatestSupportedMacOSRestoreImage(ctx context.Context, destPath string) (*progress.Reader, error) {
	url, err := GetLatestSupportedMacOSRestoreImageURL()
	if err != nil {
		return nil, err
	}
	progressReader, err := downloadRestoreImage(ctx, url, destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to download from %q: %w", url, err)
	}
	return progressReader, nil
}

// LoadMacOSRestoreImageFromPath loads a macOS restore image from a filepath on the local file system.
//
// If the imagePath parameter doesn’t refer to a local file, the system raises an exception via Objective-C.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func LoadMacOSRestoreImageFromPath(imagePath string) (retImage *MacOSRestoreImage, retErr error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	if _, err := os.Stat(imagePath); err != nil {
		return nil, err
	}

	waitCh := make(chan struct{})
	block := objc.BlockObjectError(func(imgPtr, errPtr unsafe.Pointer) {
		if err := newNSError(errPtr); err != nil {
			retErr = err
		} else {
			retImage = newMacOSRestoreImageFromPtr(imgPtr)
		}
		close(waitCh)
	})
	objc.ID(objc.GetClass("VZMacOSRestoreImage")).Send(
		objc.RegisterName("loadFileURL:completionHandler:"),
		objc.FileURL(imagePath),
		block,
	)
	<-waitCh
	block.Release()
	return retImage, retErr
}

// installProgressObserver is the Go-defined Objective-C class that observes the
// "fractionCompleted" key path of a VZMacOSInstaller's NSProgress. Its method
// reports each fraction to the func(float64) associated with the observer
// instance and removes itself once the progress finishes.
var (
	installProgressObserverClass objc.Class
	installProgressObserverOnce  sync.Once
)

func installProgressObserver() objc.Class {
	installProgressObserverOnce.Do(func() {
		cls, err := objc.DefineClass(
			"VZMacOSInstallerProgressObserverGo",
			objc.NSObjectClass(),
			[]objc.MethodDef{{
				Cmd: objc.RegisterName("observeValueForKeyPath:ofObject:change:context:"),
				Fn:  installProgressObserve,
			}},
		)
		if err != nil {
			panic("vz: failed to define install progress observer: " + err.Error())
		}
		installProgressObserverClass = cls
	})
	return installProgressObserverClass
}

func installProgressObserve(self objc.ID, _ objc.SEL, _, object, _, _ unsafe.Pointer) {
	handler, ok := objc.Associated(uintptr(self)).(func(float64))
	if !ok {
		return
	}
	id := objc.ID(uintptr(object))
	handler(objc.Send[float64](id, objc.RegisterName("fractionCompleted")))
	if objc.Send[bool](id, objc.RegisterName("finished")) {
		objc.SendVoid(object, "removeObserver:forKeyPath:", self, objc.NSString("fractionCompleted"))
	}
}

// MacOSInstaller is a struct you use to install macOS on the specified virtual machine.
type MacOSInstaller struct {
	*pointer
	observerPointer *pointer

	vm       *VirtualMachine
	progress atomic.Value
	doneCh   chan struct{}
	once     sync.Once
	err      error
}

// NewMacOSInstaller creates a new MacOSInstaller struct.
//
// A param vm is the virtual machine that the operating system will be installed onto.
// A param restoreImageIpsw is a file path indicating the macOS restore image to install.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewMacOSInstaller(vm *VirtualMachine, restoreImageIpsw string) (*MacOSInstaller, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	if _, err := os.Stat(restoreImageIpsw); err != nil {
		return nil, err
	}

	var installerPtr unsafe.Pointer
	objc.DispatchSync(vm.dispatchQueue, func() {
		installerPtr = objc.New(
			"VZMacOSInstaller", "initWithVirtualMachine:restoreImageURL:",
			objc.Ptr(vm),
			objc.FileURL(restoreImageIpsw),
		)
	})

	ret := &MacOSInstaller{
		pointer:         objc.NewPointer(installerPtr),
		observerPointer: objc.NewPointer(objc.NewObject(installProgressObserver())),
		vm:              vm,
		doneCh:          make(chan struct{}),
	}
	ret.setFractionCompleted(0)
	objc.SetFinalizer(ret, func(self *MacOSInstaller) {
		objc.Release(self.observerPointer)
		objc.Release(self)
	})
	return ret, nil
}

// Install starts installing macOS.
//
// This method starts the installation process. The VM must be in a stopped state.
// During the installation operation, pausing or stopping the VM results in an undefined behavior.
func (m *MacOSInstaller) Install(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	m.once.Do(func() {
		completionBlock := objc.BlockError(func(errPtr unsafe.Pointer) {
			if err := newNSError(errPtr); err != nil {
				m.err = err
			}
			close(m.doneCh)
		})
		observerPtr := objc.Ptr(m.observerPointer)
		objc.Associate(uintptr(observerPtr), func(v float64) {
			m.setFractionCompleted(v)
		})

		// The completion block must outlive this call (installation is
		// asynchronous). Once installation finishes (doneCh is closed) the
		// framework no longer references it, so release the block and drop the
		// observer's Go state to allow the installer to be collected.
		go func() {
			<-m.doneCh
			completionBlock.Release()
			objc.Disassociate(uintptr(observerPtr))
		}()

		objc.DispatchSync(m.vm.dispatchQueue, func() {
			objc.SendVoid(objc.Ptr(m), "installWithCompletionHandler:", completionBlock)
			progressPtr := objc.SendPtr(objc.Ptr(m), "progress")
			objc.SendVoid(
				progressPtr, "addObserver:forKeyPath:options:context:",
				observerPtr,
				objc.NSString("fractionCompleted"),
				uint(nsKeyValueObservingOptionInitial|nsKeyValueObservingOptionNew),
				unsafe.Pointer(nil),
			)
		})
	})

	select {
	case <-ctx.Done():
		progressPtr := objc.SendPtr(objc.Ptr(m), "progress")
		if objc.Send[bool](objc.ID(uintptr(progressPtr)), objc.RegisterName("isCancellable")) {
			objc.SendVoid(progressPtr, "cancel")
		}
		return ctx.Err()
	case <-m.doneCh:
	}

	return m.err
}

func (m *MacOSInstaller) setFractionCompleted(completed float64) {
	m.progress.Store(completed)
}

// FractionCompleted returns the fraction of the overall work that the install process
// completes.
func (m *MacOSInstaller) FractionCompleted() float64 {
	return m.progress.Load().(float64)
}

// Done recieves a notification that indicates the install process is completed.
func (m *MacOSInstaller) Done() <-chan struct{} { return m.doneCh }

// saveRestore issues a save/restore selector (which takes a file URL and a
// completionHandler:) on the VM's queue and waits for the completion handler.
func (v *VirtualMachine) saveRestore(sel, saveFilePath string) error {
	errCh := make(chan error, 1)
	block := completionBlockError(errCh)
	fileURL := objc.FileURL(saveFilePath)
	objc.DispatchSync(v.dispatchQueue, func() {
		objc.SendVoid(objc.Ptr(v), sel, fileURL, block)
	})
	err := <-errCh
	block.Release()
	return err
}

// SaveMachineStateToPath saves the state of a VM.
//
// You can use the contents of this file later to restore the state of the paused VM.
// This call fails if the VM isn’t in a paused state or if the Virtualization framework can’t
// save the VM. If this method fails, the framework returns an error, and the VM state remains
// unchanged.
//
// If this method is successful, the framework writes the file, and the VM state remains unchanged.
//
// Note that If you want to implement proper error handling, please make sure to call the
// `(*VirtualMachineConfiguration).ValidateSaveRestoreSupport` method before calling this method.
//
// If you want to listen status change events, use the "StateChangedNotify" method.
//
// This is only supported on macOS 14 and newer, error will
// be returned on older versions.
func (v *VirtualMachine) SaveMachineStateToPath(saveFilePath string) error {
	if err := macOSAvailable(14); err != nil {
		return err
	}
	if _, err := v.config.ValidateSaveRestoreSupport(); err != nil {
		return err
	}
	return v.saveRestore("saveMachineStateToURL:completionHandler:", saveFilePath)
}

// RestoreMachineStateFromURL restores a VM from a previously saved state.
//
// The method fails if any of the following conditions are true:
//   - The Virtualization framework can’t open or read the file.
//   - The file contents are incompatible with the current configuration.
//   - The VM you’re trying to restore isn’t in the VirtualMachineStateStopped state.
//
// If this method fails, the framework returns an error, and the VM state doesn’t change.
//
// If this method is successful, the framework restores the VM and places it in the paused state.
//
// Note that If you want to implement proper error handling, please make sure to call the
// `(*VirtualMachineConfiguration).ValidateSaveRestoreSupport` method before calling this method.
//
// If you want to listen status change events, use the "StateChangedNotify" method.
//
// This is only supported on macOS 14 and newer, error will
// be returned on older versions.
func (v *VirtualMachine) RestoreMachineStateFromURL(saveFilePath string) error {
	if err := macOSAvailable(14); err != nil {
		return err
	}
	if _, err := v.config.ValidateSaveRestoreSupport(); err != nil {
		return err
	}
	return v.saveRestore("restoreMachineStateFromURL:completionHandler:", saveFilePath)
}
