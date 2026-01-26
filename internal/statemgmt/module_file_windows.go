// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package statemgmt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	advapi32                     = syscall.NewLazyDLL("advapi32.dll")
	procGetSecurityInfo          = advapi32.NewProc("GetSecurityInfo")
	procSetSecurityInfo          = advapi32.NewProc("SetSecurityInfo")
	procLookupAccountSidW        = advapi32.NewProc("LookupAccountSidW")
	procLookupAccountNameW       = advapi32.NewProc("LookupAccountNameW")
	procGetLengthSid             = advapi32.NewProc("GetLengthSid")
	procConvertSidToStringSid    = advapi32.NewProc("ConvertSidToStringSidW")
	procConvertStringSidToSid    = advapi32.NewProc("ConvertStringSidToSidW")
	procGetAce                   = advapi32.NewProc("GetAce")
	procGetAclInformation        = advapi32.NewProc("GetAclInformation")
	procInitializeAcl            = advapi32.NewProc("InitializeAcl")
	procAddAccessAllowedAceEx    = advapi32.NewProc("AddAccessAllowedAceEx")
	procAddAccessDeniedAceEx     = advapi32.NewProc("AddAccessDeniedAceEx")
	procSetNamedSecurityInfoW    = advapi32.NewProc("SetNamedSecurityInfoW")
	procGetNamedSecurityInfoW    = advapi32.NewProc("GetNamedSecurityInfoW")
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procLocalFree                = kernel32.NewProc("LocalFree")
	procGetFileAttributesW       = kernel32.NewProc("GetFileAttributesW")
	procSetFileAttributesW       = kernel32.NewProc("SetFileAttributesW")
	procGetLongPathNameW         = kernel32.NewProc("GetLongPathNameW")
	procGetShortPathNameW        = kernel32.NewProc("GetShortPathNameW")
	procGetFullPathNameW         = kernel32.NewProc("GetFullPathNameW")
)

const (
	SE_FILE_OBJECT              = 1
	OWNER_SECURITY_INFORMATION  = 0x00000001
	GROUP_SECURITY_INFORMATION  = 0x00000002
	DACL_SECURITY_INFORMATION   = 0x00000004
	SACL_SECURITY_INFORMATION   = 0x00000008
	PROTECTED_DACL_SECURITY_INFORMATION = 0x80000000

	// ACL revision
	ACL_REVISION = 2

	// Access mask flags
	DELETE                   = 0x00010000
	READ_CONTROL             = 0x00020000
	WRITE_DAC                = 0x00040000
	WRITE_OWNER              = 0x00080000
	SYNCHRONIZE              = 0x00100000
	STANDARD_RIGHTS_REQUIRED = DELETE | READ_CONTROL | WRITE_DAC | WRITE_OWNER
	STANDARD_RIGHTS_READ     = READ_CONTROL
	STANDARD_RIGHTS_WRITE    = READ_CONTROL
	STANDARD_RIGHTS_EXECUTE  = READ_CONTROL
	STANDARD_RIGHTS_ALL      = 0x001F0000
	SPECIFIC_RIGHTS_ALL      = 0x0000FFFF
	GENERIC_READ             = 0x80000000
	GENERIC_WRITE            = 0x40000000
	GENERIC_EXECUTE          = 0x20000000
	GENERIC_ALL              = 0x10000000

	// File-specific access rights
	FILE_READ_DATA        = 0x0001
	FILE_WRITE_DATA       = 0x0002
	FILE_APPEND_DATA      = 0x0004
	FILE_READ_EA          = 0x0008
	FILE_WRITE_EA         = 0x0010
	FILE_EXECUTE          = 0x0020
	FILE_DELETE_CHILD     = 0x0040
	FILE_READ_ATTRIBUTES  = 0x0080
	FILE_WRITE_ATTRIBUTES = 0x0100

	FILE_ALL_ACCESS     = STANDARD_RIGHTS_REQUIRED | SYNCHRONIZE | 0x1FF
	FILE_GENERIC_READ   = STANDARD_RIGHTS_READ | FILE_READ_DATA | FILE_READ_ATTRIBUTES | FILE_READ_EA | SYNCHRONIZE
	FILE_GENERIC_WRITE  = STANDARD_RIGHTS_WRITE | FILE_WRITE_DATA | FILE_WRITE_ATTRIBUTES | FILE_WRITE_EA | FILE_APPEND_DATA | SYNCHRONIZE
	FILE_GENERIC_EXECUTE = STANDARD_RIGHTS_EXECUTE | FILE_READ_ATTRIBUTES | FILE_EXECUTE | SYNCHRONIZE

	// ACE types
	ACCESS_ALLOWED_ACE_TYPE = 0
	ACCESS_DENIED_ACE_TYPE  = 1

	// ACE flags
	OBJECT_INHERIT_ACE         = 0x01
	CONTAINER_INHERIT_ACE      = 0x02
	NO_PROPAGATE_INHERIT_ACE   = 0x04
	INHERIT_ONLY_ACE           = 0x08
	INHERITED_ACE              = 0x10

	// File attributes
	FILE_ATTRIBUTE_READONLY            = 0x00000001
	FILE_ATTRIBUTE_HIDDEN              = 0x00000002
	FILE_ATTRIBUTE_SYSTEM              = 0x00000004
	FILE_ATTRIBUTE_DIRECTORY           = 0x00000010
	FILE_ATTRIBUTE_ARCHIVE             = 0x00000020
	FILE_ATTRIBUTE_DEVICE              = 0x00000040
	FILE_ATTRIBUTE_NORMAL              = 0x00000080
	FILE_ATTRIBUTE_TEMPORARY           = 0x00000100
	FILE_ATTRIBUTE_SPARSE_FILE         = 0x00000200
	FILE_ATTRIBUTE_REPARSE_POINT       = 0x00000400
	FILE_ATTRIBUTE_COMPRESSED          = 0x00000800
	FILE_ATTRIBUTE_OFFLINE             = 0x00001000
	FILE_ATTRIBUTE_NOT_CONTENT_INDEXED = 0x00002000
	FILE_ATTRIBUTE_ENCRYPTED           = 0x00004000
	INVALID_FILE_ATTRIBUTES            = 0xFFFFFFFF
)

// ACE_HEADER represents a Windows ACE header
type ACE_HEADER struct {
	AceType  byte
	AceFlags byte
	AceSize  uint16
}

// ACCESS_ALLOWED_ACE represents a Windows access allowed ACE
type ACCESS_ALLOWED_ACE struct {
	Header   ACE_HEADER
	Mask     uint32
	SidStart uint32
}

// ACL_SIZE_INFORMATION represents ACL size information
type ACL_SIZE_INFORMATION struct {
	AceCount      uint32
	AclBytesInUse uint32
	AclBytesFree  uint32
}

// WindowsACL represents Windows Access Control List
type WindowsACL struct {
	Owner   string            // Owner SID or name
	Group   string            // Primary group SID or name
	Entries []WindowsACE      // Access Control Entries
}

// WindowsACE represents a single Access Control Entry
type WindowsACE struct {
	Type       string // "allow" or "deny"
	Principal  string // SID or account name
	Access     uint32 // Access mask
	Flags      byte   // Inheritance flags
	Inherited  bool   // Whether this ACE is inherited
}

// WindowsFileAttributes represents Windows file attributes
type WindowsFileAttributes struct {
	ReadOnly            bool
	Hidden              bool
	System              bool
	Archive             bool
	Compressed          bool
	Encrypted           bool
	NotContentIndexed   bool
	Temporary           bool
	Offline             bool
	ReparsePoint        bool
	SparseFile          bool
	RawValue            uint32
}

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
	path = NormalizePath(path)
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}

	// Get security information
	var pSidOwner *byte
	var pSecurityDescriptor uintptr

	ret, _, _ := procGetNamedSecurityInfoW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
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

	return lookupAccountBySid(pSidOwner)
}

// lookupAccountBySid converts a SID to an account name
func lookupAccountBySid(sid *byte) (string, error) {
	var nameLen uint32 = 256
	var domainLen uint32 = 256
	var use uint32

	nameBuf := make([]uint16, nameLen)
	domainBuf := make([]uint16, domainLen)

	ret, _, _ := procLookupAccountSidW.Call(
		0, // lpSystemName (local)
		uintptr(unsafe.Pointer(sid)),
		uintptr(unsafe.Pointer(&nameBuf[0])),
		uintptr(unsafe.Pointer(&nameLen)),
		uintptr(unsafe.Pointer(&domainBuf[0])),
		uintptr(unsafe.Pointer(&domainLen)),
		uintptr(unsafe.Pointer(&use)),
	)
	if ret == 0 {
		// Try to get SID string as fallback
		return getSidString(sid), nil
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

// =============================================================================
// Phase 4: Windows ACL Support
// =============================================================================

// GetWindowsACL retrieves the full ACL for a file or directory
func GetWindowsACL(path string) (*WindowsACL, error) {
	path = NormalizePath(path)
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}

	var pSidOwner, pSidGroup *byte
	var pDacl uintptr
	var pSecurityDescriptor uintptr

	ret, _, _ := procGetNamedSecurityInfoW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		SE_FILE_OBJECT,
		OWNER_SECURITY_INFORMATION|GROUP_SECURITY_INFORMATION|DACL_SECURITY_INFORMATION,
		uintptr(unsafe.Pointer(&pSidOwner)),
		uintptr(unsafe.Pointer(&pSidGroup)),
		uintptr(unsafe.Pointer(&pDacl)),
		0, // pSacl
		uintptr(unsafe.Pointer(&pSecurityDescriptor)),
	)
	if ret != 0 {
		return nil, fmt.Errorf("GetNamedSecurityInfo failed: %w", syscall.Errno(ret))
	}
	defer procLocalFree.Call(pSecurityDescriptor)

	acl := &WindowsACL{
		Entries: make([]WindowsACE, 0),
	}

	// Get owner
	if pSidOwner != nil {
		ownerName, err := lookupAccountBySid(pSidOwner)
		if err == nil {
			acl.Owner = ownerName
		} else {
			acl.Owner = getSidString(pSidOwner)
		}
	}

	// Get primary group
	if pSidGroup != nil {
		groupName, err := lookupAccountBySid(pSidGroup)
		if err == nil {
			acl.Group = groupName
		} else {
			acl.Group = getSidString(pSidGroup)
		}
	}

	// Get ACEs from DACL
	if pDacl != 0 {
		var aclInfo ACL_SIZE_INFORMATION
		ret, _, _ := procGetAclInformation.Call(
			pDacl,
			uintptr(unsafe.Pointer(&aclInfo)),
			unsafe.Sizeof(aclInfo),
			2, // AclSizeInformation
		)
		if ret != 0 {
			for i := uint32(0); i < aclInfo.AceCount; i++ {
				var pAce uintptr
				ret, _, _ := procGetAce.Call(pDacl, uintptr(i), uintptr(unsafe.Pointer(&pAce)))
				if ret == 0 {
					continue
				}

				header := (*ACE_HEADER)(unsafe.Pointer(pAce))
				ace := WindowsACE{
					Flags:     header.AceFlags,
					Inherited: (header.AceFlags & INHERITED_ACE) != 0,
				}

				switch header.AceType {
				case ACCESS_ALLOWED_ACE_TYPE:
					ace.Type = "allow"
					allowAce := (*ACCESS_ALLOWED_ACE)(unsafe.Pointer(pAce))
					ace.Access = allowAce.Mask
					sidPtr := (*byte)(unsafe.Pointer(uintptr(pAce) + unsafe.Offsetof(allowAce.SidStart)))
					principal, _ := lookupAccountBySid(sidPtr)
					ace.Principal = principal
				case ACCESS_DENIED_ACE_TYPE:
					ace.Type = "deny"
					denyAce := (*ACCESS_ALLOWED_ACE)(unsafe.Pointer(pAce)) // Same structure as allow
					ace.Access = denyAce.Mask
					sidPtr := (*byte)(unsafe.Pointer(uintptr(pAce) + unsafe.Offsetof(denyAce.SidStart)))
					principal, _ := lookupAccountBySid(sidPtr)
					ace.Principal = principal
				}

				if ace.Principal != "" {
					acl.Entries = append(acl.Entries, ace)
				}
			}
		}
	}

	return acl, nil
}

// SetWindowsOwner sets the owner of a file or directory
func SetWindowsOwner(path, owner string) error {
	path = NormalizePath(path)
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}

	// Convert account name to SID
	sid, err := lookupAccountName(owner)
	if err != nil {
		return fmt.Errorf("failed to lookup account %s: %w", owner, err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(sid)))

	ret, _, _ := procSetNamedSecurityInfoW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		SE_FILE_OBJECT,
		OWNER_SECURITY_INFORMATION,
		uintptr(unsafe.Pointer(sid)),
		0, // pSidGroup
		0, // pDacl
		0, // pSacl
	)
	if ret != 0 {
		return fmt.Errorf("SetNamedSecurityInfo failed: %w", syscall.Errno(ret))
	}

	return nil
}

// lookupAccountName converts an account name to a SID
func lookupAccountName(name string) (*byte, error) {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}

	var sidSize uint32 = 0
	var domainSize uint32 = 0
	var use uint32

	// First call to get buffer sizes
	procLookupAccountNameW.Call(
		0,
		uintptr(unsafe.Pointer(namePtr)),
		0,
		uintptr(unsafe.Pointer(&sidSize)),
		0,
		uintptr(unsafe.Pointer(&domainSize)),
		uintptr(unsafe.Pointer(&use)),
	)

	if sidSize == 0 {
		return nil, fmt.Errorf("account not found: %s", name)
	}

	sid := make([]byte, sidSize)
	domain := make([]uint16, domainSize)

	ret, _, _ := procLookupAccountNameW.Call(
		0,
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(&sid[0])),
		uintptr(unsafe.Pointer(&sidSize)),
		uintptr(unsafe.Pointer(&domain[0])),
		uintptr(unsafe.Pointer(&domainSize)),
		uintptr(unsafe.Pointer(&use)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("LookupAccountName failed")
	}

	return &sid[0], nil
}

// AccessMaskToString converts a Windows access mask to a readable string
func AccessMaskToString(mask uint32) string {
	var perms []string

	if mask&FILE_ALL_ACCESS == FILE_ALL_ACCESS {
		return "FullControl"
	}

	if mask&FILE_GENERIC_READ == FILE_GENERIC_READ {
		perms = append(perms, "Read")
	}
	if mask&FILE_GENERIC_WRITE == FILE_GENERIC_WRITE {
		perms = append(perms, "Write")
	}
	if mask&FILE_GENERIC_EXECUTE == FILE_GENERIC_EXECUTE {
		perms = append(perms, "Execute")
	}
	if mask&DELETE != 0 {
		perms = append(perms, "Delete")
	}
	if mask&WRITE_DAC != 0 {
		perms = append(perms, "ChangePermissions")
	}
	if mask&WRITE_OWNER != 0 {
		perms = append(perms, "TakeOwnership")
	}

	if len(perms) == 0 {
		return fmt.Sprintf("0x%08X", mask)
	}
	return strings.Join(perms, ",")
}

// StringToAccessMask converts a permission string to access mask
func StringToAccessMask(perm string) uint32 {
	perm = strings.ToLower(perm)
	switch perm {
	case "fullcontrol", "full", "f":
		return FILE_ALL_ACCESS
	case "modify", "m":
		return FILE_GENERIC_READ | FILE_GENERIC_WRITE | FILE_GENERIC_EXECUTE | DELETE
	case "readandexecute", "rx":
		return FILE_GENERIC_READ | FILE_GENERIC_EXECUTE
	case "read", "r":
		return FILE_GENERIC_READ
	case "write", "w":
		return FILE_GENERIC_WRITE
	case "execute", "x":
		return FILE_GENERIC_EXECUTE
	default:
		return 0
	}
}

// =============================================================================
// Phase 4: Windows Path Handling
// =============================================================================

// NormalizePath normalizes a Windows path, handling UNC and long paths
func NormalizePath(path string) string {
	if path == "" {
		return path
	}

	// Convert forward slashes to backslashes
	path = strings.ReplaceAll(path, "/", "\\")

	// Already has long path prefix
	if strings.HasPrefix(path, `\\?\`) {
		return path
	}

	// UNC path - add long path UNC prefix if needed
	if strings.HasPrefix(path, `\\`) {
		// Check if it's long enough to need prefix
		if len(path) > 260 {
			return `\\?\UNC\` + path[2:]
		}
		return path
	}

	// Regular path - make absolute and add prefix if needed
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err == nil {
			path = absPath
		}
	}

	// Add long path prefix for paths approaching limit
	if len(path) > 248 {
		return `\\?\` + path
	}

	return path
}

// IsUNCPath checks if a path is a UNC path
func IsUNCPath(path string) bool {
	return strings.HasPrefix(path, `\\`) && !strings.HasPrefix(path, `\\?\`)
}

// IsLongPath checks if a path has the long path prefix
func IsLongPath(path string) bool {
	return strings.HasPrefix(path, `\\?\`)
}

// GetLongPathName retrieves the long path name for a path
func GetLongPathName(path string) (string, error) {
	path = NormalizePath(path)
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}

	// Get required buffer size
	n, _, _ := procGetLongPathNameW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		0,
	)
	if n == 0 {
		return path, nil // Return original if can't convert
	}

	buf := make([]uint16, n)
	n, _, err = procGetLongPathNameW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(n),
	)
	if n == 0 {
		return path, nil
	}

	return syscall.UTF16ToString(buf), nil
}

// GetShortPathName retrieves the short (8.3) path name for a path
func GetShortPathName(path string) (string, error) {
	path = NormalizePath(path)
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}

	// Get required buffer size
	n, _, _ := procGetShortPathNameW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		0,
	)
	if n == 0 {
		return path, nil
	}

	buf := make([]uint16, n)
	n, _, _ = procGetShortPathNameW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(n),
	)
	if n == 0 {
		return path, nil
	}

	return syscall.UTF16ToString(buf), nil
}

// GetFullPath returns the full path for a relative path
func GetFullPath(path string) (string, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}

	// Get required buffer size
	n, _, _ := procGetFullPathNameW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		0,
		0,
	)
	if n == 0 {
		return "", fmt.Errorf("GetFullPathName failed")
	}

	buf := make([]uint16, n)
	n, _, _ = procGetFullPathNameW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(n),
		uintptr(unsafe.Pointer(&buf[0])),
		0,
	)
	if n == 0 {
		return "", fmt.Errorf("GetFullPathName failed")
	}

	return syscall.UTF16ToString(buf), nil
}

// ParseDriveLetter extracts the drive letter from a path
func ParseDriveLetter(path string) (string, string) {
	if len(path) >= 2 && path[1] == ':' {
		return path[:2], path[2:]
	}
	return "", path
}

// =============================================================================
// Phase 4: Windows File Attributes
// =============================================================================

// GetFileAttributes retrieves Windows file attributes
func GetFileAttributes(path string) (*WindowsFileAttributes, error) {
	path = NormalizePath(path)
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}

	attrs, _, err := procGetFileAttributesW.Call(uintptr(unsafe.Pointer(pathPtr)))
	if attrs == INVALID_FILE_ATTRIBUTES {
		return nil, fmt.Errorf("GetFileAttributes failed: %w", err)
	}

	return &WindowsFileAttributes{
		ReadOnly:          (attrs & FILE_ATTRIBUTE_READONLY) != 0,
		Hidden:            (attrs & FILE_ATTRIBUTE_HIDDEN) != 0,
		System:            (attrs & FILE_ATTRIBUTE_SYSTEM) != 0,
		Archive:           (attrs & FILE_ATTRIBUTE_ARCHIVE) != 0,
		Compressed:        (attrs & FILE_ATTRIBUTE_COMPRESSED) != 0,
		Encrypted:         (attrs & FILE_ATTRIBUTE_ENCRYPTED) != 0,
		NotContentIndexed: (attrs & FILE_ATTRIBUTE_NOT_CONTENT_INDEXED) != 0,
		Temporary:         (attrs & FILE_ATTRIBUTE_TEMPORARY) != 0,
		Offline:           (attrs & FILE_ATTRIBUTE_OFFLINE) != 0,
		ReparsePoint:      (attrs & FILE_ATTRIBUTE_REPARSE_POINT) != 0,
		SparseFile:        (attrs & FILE_ATTRIBUTE_SPARSE_FILE) != 0,
		RawValue:          uint32(attrs),
	}, nil
}

// SetFileAttributes sets Windows file attributes
func SetFileAttributes(path string, attrs *WindowsFileAttributes) error {
	path = NormalizePath(path)
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}

	// Build attribute flags
	var flags uint32

	if attrs.ReadOnly {
		flags |= FILE_ATTRIBUTE_READONLY
	}
	if attrs.Hidden {
		flags |= FILE_ATTRIBUTE_HIDDEN
	}
	if attrs.System {
		flags |= FILE_ATTRIBUTE_SYSTEM
	}
	if attrs.Archive {
		flags |= FILE_ATTRIBUTE_ARCHIVE
	}
	if attrs.NotContentIndexed {
		flags |= FILE_ATTRIBUTE_NOT_CONTENT_INDEXED
	}
	if attrs.Temporary {
		flags |= FILE_ATTRIBUTE_TEMPORARY
	}
	if attrs.Offline {
		flags |= FILE_ATTRIBUTE_OFFLINE
	}

	// If no flags set, use NORMAL
	if flags == 0 {
		flags = FILE_ATTRIBUTE_NORMAL
	}

	ret, _, err := procSetFileAttributesW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(flags),
	)
	if ret == 0 {
		return fmt.Errorf("SetFileAttributes failed: %w", err)
	}

	return nil
}

// SetFileHidden sets or clears the hidden attribute
func SetFileHidden(path string, hidden bool) error {
	attrs, err := GetFileAttributes(path)
	if err != nil {
		return err
	}
	attrs.Hidden = hidden
	return SetFileAttributes(path, attrs)
}

// SetFileReadOnly sets or clears the read-only attribute
func SetFileReadOnly(path string, readOnly bool) error {
	attrs, err := GetFileAttributes(path)
	if err != nil {
		return err
	}
	attrs.ReadOnly = readOnly
	return SetFileAttributes(path, attrs)
}

// SetFileSystem sets or clears the system attribute
func SetFileSystem(path string, system bool) error {
	attrs, err := GetFileAttributes(path)
	if err != nil {
		return err
	}
	attrs.System = system
	return SetFileAttributes(path, attrs)
}

// SetFileArchive sets or clears the archive attribute
func SetFileArchive(path string, archive bool) error {
	attrs, err := GetFileAttributes(path)
	if err != nil {
		return err
	}
	attrs.Archive = archive
	return SetFileAttributes(path, attrs)
}

// AttributesToString converts file attributes to a readable string
func AttributesToString(attrs *WindowsFileAttributes) string {
	var parts []string
	if attrs.ReadOnly {
		parts = append(parts, "ReadOnly")
	}
	if attrs.Hidden {
		parts = append(parts, "Hidden")
	}
	if attrs.System {
		parts = append(parts, "System")
	}
	if attrs.Archive {
		parts = append(parts, "Archive")
	}
	if attrs.Compressed {
		parts = append(parts, "Compressed")
	}
	if attrs.Encrypted {
		parts = append(parts, "Encrypted")
	}
	if attrs.Temporary {
		parts = append(parts, "Temporary")
	}
	if attrs.Offline {
		parts = append(parts, "Offline")
	}
	if attrs.SparseFile {
		parts = append(parts, "Sparse")
	}
	if attrs.ReparsePoint {
		parts = append(parts, "ReparsePoint")
	}
	if attrs.NotContentIndexed {
		parts = append(parts, "NotIndexed")
	}

	if len(parts) == 0 {
		return "Normal"
	}
	return strings.Join(parts, ",")
}
