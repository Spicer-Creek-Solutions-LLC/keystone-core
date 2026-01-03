//go:build windows

package statemgmt

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	advapi32                  = syscall.NewLazyDLL("advapi32.dll")
	procGetSecurityInfo       = advapi32.NewProc("GetSecurityInfo")
	procLookupAccountSidW     = advapi32.NewProc("LookupAccountSidW")
	procGetLengthSid          = advapi32.NewProc("GetLengthSid")
	procConvertSidToStringSid = advapi32.NewProc("ConvertSidToStringSidW")
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procLocalFree             = kernel32.NewProc("LocalFree")
)

const (
	SE_FILE_OBJECT           = 1
	OWNER_SECURITY_INFORMATION = 0x00000001
)

// getFileOwnership returns a pseudo UID/GID for Windows file ownership.
// Windows uses ACLs, not Unix-style UID/GID. This returns:
// - uid: hash of owner SID (or 0 if unable to retrieve)
// - gid: always 0 (Windows doesn't have Unix-style groups)
// - ok: true if owner information was successfully retrieved
func getFileOwnership(info os.FileInfo) (uid, gid uint32, ok bool) {
	// Get the file path from the FileInfo
	// Note: os.FileInfo doesn't expose the path, so we can't use this approach
	// Instead, we return a placeholder indicating Windows ownership isn't
	// directly comparable to Unix UID/GID
	//
	// For actual file ownership checks, use getFileOwnerName() with the file path
	return 0, 0, false
}

// getFileOwnerName returns the owner name of a file on Windows.
// This is the Windows-native way to check file ownership.
func getFileOwnerName(path string) (owner string, err error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}

	// Open the file to get a handle
	handle, err := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS, // Required for directories
		0,
	)
	if err != nil {
		return "", err
	}
	defer syscall.CloseHandle(handle)

	// Get security information
	var pSidOwner *byte
	var pSecurityDescriptor uintptr

	ret, _, _ := procGetSecurityInfo.Call(
		uintptr(handle),
		SE_FILE_OBJECT,
		OWNER_SECURITY_INFORMATION,
		uintptr(unsafe.Pointer(&pSidOwner)),
		0, // pSidGroup
		0, // pDacl
		0, // pSacl
		uintptr(unsafe.Pointer(&pSecurityDescriptor)),
	)
	if ret != 0 {
		return "", syscall.Errno(ret)
	}
	defer procLocalFree.Call(pSecurityDescriptor)

	if pSidOwner == nil {
		return "", nil
	}

	// Lookup account name from SID
	var nameLen uint32 = 256
	var domainLen uint32 = 256
	var use uint32

	nameBuf := make([]uint16, nameLen)
	domainBuf := make([]uint16, domainLen)

	ret, _, _ = procLookupAccountSidW.Call(
		0, // lpSystemName (local)
		uintptr(unsafe.Pointer(pSidOwner)),
		uintptr(unsafe.Pointer(&nameBuf[0])),
		uintptr(unsafe.Pointer(&nameLen)),
		uintptr(unsafe.Pointer(&domainBuf[0])),
		uintptr(unsafe.Pointer(&domainLen)),
		uintptr(unsafe.Pointer(&use)),
	)
	if ret == 0 {
		// Try to get SID string as fallback
		return getSidString(pSidOwner), nil
	}

	name := syscall.UTF16ToString(nameBuf[:nameLen])
	domain := syscall.UTF16ToString(domainBuf[:domainLen])

	if domain != "" {
		return domain + "\\" + name, nil
	}
	return name, nil
}

// getSidString converts a SID pointer to its string representation (e.g., S-1-5-21-...)
func getSidString(sid *byte) string {
	var stringSid *uint16
	ret, _, _ := procConvertSidToStringSid.Call(
		uintptr(unsafe.Pointer(sid)),
		uintptr(unsafe.Pointer(&stringSid)),
	)
	if ret == 0 {
		return ""
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(stringSid)))

	// Convert UTF16 string to Go string
	return utf16PtrToString(stringSid)
}

// utf16PtrToString converts a null-terminated UTF16 string pointer to a Go string
func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	// Find null terminator
	end := unsafe.Pointer(p)
	n := 0
	for *(*uint16)(end) != 0 {
		end = unsafe.Pointer(uintptr(end) + 2)
		n++
	}
	// Convert to slice
	s := (*[1 << 20]uint16)(unsafe.Pointer(p))[:n:n]
	return syscall.UTF16ToString(s)
}

// isOwnershipSupported returns false on Windows.
// Windows uses ACLs, not Unix-style UID/GID. While we can read the owner
// via getFileOwnerName(), the Unix-style ownership model doesn't apply.
// Use getFileOwnerName() for Windows-native ownership checks.
func isOwnershipSupported() bool {
	// Return false because Unix-style numeric UID/GID isn't supported.
	// Callers should use getFileOwnerName() for Windows ownership.
	return false
}

// isSymlinkFullySupported returns false on Windows.
// Symlinks on Windows require elevated privileges (SeCreateSymbolicLinkPrivilege)
// and behave differently than Unix symlinks. Directory symlinks are especially
// problematic. For safety, we report symlinks as not fully supported.
func isSymlinkFullySupported() bool {
	return false
}

// checkWindowsOwnership checks if the file is owned by the specified owner.
// On Windows, owner should be in the format "DOMAIN\username" or just "username".
func checkWindowsOwnership(path string, expectedOwner string) (bool, error) {
	actualOwner, err := getFileOwnerName(path)
	if err != nil {
		return false, err
	}

	// Case-insensitive comparison for Windows
	return equalFoldASCII(actualOwner, expectedOwner), nil
}

// equalFoldASCII performs case-insensitive ASCII string comparison
func equalFoldASCII(s, t string) bool {
	if len(s) != len(t) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if toLowerASCII(s[i]) != toLowerASCII(t[i]) {
			return false
		}
	}
	return true
}

// toLowerASCII converts an ASCII character to lowercase
func toLowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}
