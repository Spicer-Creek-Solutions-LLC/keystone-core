package main

import "time"

const (
	filesTimeoutShort     = 30 * time.Second
	filesTimeoutDownload  = 5 * time.Minute
	filesTimeoutUpload    = 10 * time.Minute
	filesTimeoutSync      = 30 * time.Minute
	filesTimeoutAdmin     = 30 * time.Second
	filesTimeoutAdminLong = 5 * time.Minute
	filesTimeoutAdminSync = 30 * time.Minute
)
