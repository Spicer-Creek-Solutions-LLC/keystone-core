// Hello World TitanAnvil Module (Go/TinyGo)
//
// This example module demonstrates the TitanAnvil Go SDK by:
// - Getting the CPU make and model
// - Computing a SHA256 hash of the CPU information
// - Writing the results to a file in the temp directory
// - Returning the results as JSON

package main

import (
	"encoding/json"
	"fmt"
	"runtime"

	titansdk "github.com/titananvil/titan-module-sdk-go"
)

type HelloWorldResult struct {
	CPUInfo  string `json:"cpu_info"`
	Hash     string `json:"hash"`
	FilePath string `json:"file_path"`
}

func main() {
	titansdk.LogInfo("Hello World Go module starting")

	// Get CPU information
	titansdk.LogInfo("Getting CPU information...")
	cpuInfo, err := titansdk.GetCPUInfo()
	if err != nil {
		titansdk.LogError(fmt.Sprintf("Failed to get CPU info: %v", err))
		return
	}
	titansdk.LogInfo(fmt.Sprintf("CPU: %s", cpuInfo))

	// Compute SHA256 hash of CPU info
	titansdk.LogInfo("Computing SHA256 hash...")
	hash, err := titansdk.SHA256String(cpuInfo)
	if err != nil {
		titansdk.LogError(fmt.Sprintf("Failed to compute hash: %v", err))
		return
	}
	titansdk.LogInfo(fmt.Sprintf("Hash: %s", hash))

	// Determine temp directory path (cross-platform)
	var tempDir string
	if runtime.GOOS == "windows" {
		tempDir = "C:\\Windows\\Temp"
	} else {
		tempDir = "/tmp"
	}

	filePath := fmt.Sprintf("%s/hello-from-titananvil-go.txt", tempDir)

	// Create file contents
	contents := fmt.Sprintf("CPU: %s\nSHA256: %s\n", cpuInfo, hash)

	// Write to file
	titansdk.LogInfo(fmt.Sprintf("Writing to file: %s", filePath))
	if err := titansdk.WriteString(filePath, contents); err != nil {
		titansdk.LogError(fmt.Sprintf("Failed to write file: %v", err))
		return
	}
	titansdk.LogInfo("File written successfully")

	// Return results as JSON
	result := HelloWorldResult{
		CPUInfo:  cpuInfo,
		Hash:     hash,
		FilePath: filePath,
	}

	jsonData, err := json.Marshal(result)
	if err != nil {
		titansdk.LogError(fmt.Sprintf("Failed to serialize result: %v", err))
		return
	}

	titansdk.LogInfo(fmt.Sprintf("Result: %s", string(jsonData)))
	titansdk.LogInfo("Hello World Go module completed successfully")
}
