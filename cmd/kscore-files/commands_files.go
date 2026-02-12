// Package main implements file management commands for kscore-files.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/cli/output"
	"github.com/shawnbutts/keystone-core/internal/files"
)

// newFilesListCmd creates the list command.
func newFilesListCmd() *cobra.Command {
	var (
		namespace string
		recursive bool
		outputFmt string
	)

	cmd := &cobra.Command{
		Use:   "list [path]",
		Short: "List files in the distribution system",
		Long: `List files in a namespace or path.

Examples:
  kscore-files list /
  kscore-files list /packages --namespace myapp
  kscore-files list /configs --recursive`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/"
			if len(args) > 0 {
				path = args[0]
			}

			client, cleanup, err := createClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutShort)
			defer cancel()

			result, err := client.ListFiles(ctx, path, &files.ListFilesOptions{
				Namespace: namespace,
				Recursive: recursive,
			})
			if err != nil {
				return fmt.Errorf("failed to list files: %w", err)
			}

			format, err := output.ParseFormat(outputFmt)
			if err != nil {
				return err
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, result)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, result)
			case output.FormatTable, output.FormatText:
				if len(result.Entries) == 0 {
					fmt.Println("No files found")
					return nil
				}

				rows := make([][]string, 0, len(result.Entries))
				for _, entry := range result.Entries {
					entryType := "file"
					if entry.IsDir {
						entryType = "dir"
					}
					rows = append(rows, []string{
						entryType,
						formatSize(entry.Size),
						entry.ModTime.Format("2006-01-02 15:04:05"),
						entry.Path,
					})
				}

				table := &output.Table{
					Headers: []string{"TYPE", "SIZE", "MODIFIED", "PATH"},
					Rows:    rows,
				}
				if err := output.WriteTable(os.Stdout, table); err != nil {
					return err
				}
				fmt.Printf("\nTotal: %d entries\n", len(result.Entries))
				return nil
			default:
				return fmt.Errorf("unsupported output format: %s", outputFmt)
			}
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Namespace to list from")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "List recursively")
	cmd.Flags().StringVarP(&outputFmt, "output", "o", "table", "Output format (table, text, json, yaml)")

	return cmd
}

// newFilesGetCmd creates the get command.
func newFilesGetCmd() *cobra.Command {
	var (
		namespace  string
		version    string
		checksum   string
		outputPath string
	)

	cmd := &cobra.Command{
		Use:     "get <path>",
		Aliases: []string{"pull"},
		Short:   "Download a file from the distribution system",
		Long: `Download a file to the local filesystem.

Aliases: pull (for compatibility with kscorectl files pull)

Examples:
  kscore-files get /packages/nginx-1.20.rpm
  kscore-files get /configs/app.yaml --output /etc/app/app.yaml
  kscore-files get /binaries/myapp --version v1.2.0
  kscorectl files pull /packages/nginx-1.20.rpm`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			client, cleanup, err := createClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutDownload)
			defer cancel()

			opts := &files.GetFileOptions{
				Version:  version,
				Checksum: checksum,
			}

			// Prepend namespace to path if specified
			requestPath := path
			if namespace != "" {
				requestPath = namespace + "/" + path
			}

			result, err := client.GetFile(ctx, requestPath, opts)
			if err != nil {
				return fmt.Errorf("failed to get file: %w", err)
			}

			// Determine output path
			destPath := outputPath
			if destPath == "" {
				destPath = filepath.Base(path)
			}

			// Copy the file to destination
			srcFile, err := os.Open(result.LocalPath)
			if err != nil {
				return fmt.Errorf("failed to open downloaded file: %w", err)
			}
			defer srcFile.Close()

			// Create destination directory if needed
			if dir := filepath.Dir(destPath); dir != "." {
				//nolint:gosec // G301: destination directory needs to be accessible by users
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return fmt.Errorf("failed to create directory: %w", err)
				}
			}

			destFile, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("failed to create destination file: %w", err)
			}
			defer destFile.Close()

			written, err := io.Copy(destFile, srcFile)
			if err != nil {
				return fmt.Errorf("failed to copy file: %w", err)
			}

			fmt.Printf("Downloaded %s (%s) to %s\n", path, formatSize(written), destPath)
			if result.Checksum != "" {
				fmt.Printf("Checksum: %s\n", result.Checksum)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Namespace")
	cmd.Flags().StringVarP(&version, "version", "V", "", "File version")
	cmd.Flags().StringVar(&checksum, "checksum", "", "Expected checksum")
	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path")

	return cmd
}

// newFilesPutCmd creates the put command.
func newFilesPutCmd() *cobra.Command {
	var (
		namespace   string
		recursive   bool
		createDirs  bool
		contentType string
		dryRun      bool
	)

	cmd := &cobra.Command{
		Use:     "put <source> <destination>",
		Aliases: []string{"push"},
		Short:   "Upload a file to the distribution system",
		Long: `Upload a local file or directory to the distribution system.

Aliases: push (for compatibility with kscorectl files push)

Examples:
  kscore-files put ./nginx.rpm /packages/nginx-1.20.rpm
  kscore-files put ./configs/ /configs/ --recursive
  kscore-files put ./app.yaml /configs/app.yaml --namespace myapp
  kscorectl files push ./nginx.rpm /packages/nginx-1.20.rpm`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]
			dest := args[1]

			// Check if source is a directory
			info, err := os.Stat(source)
			if err != nil {
				return fmt.Errorf("failed to stat source: %w", err)
			}

			if info.IsDir() {
				if !recursive {
					return fmt.Errorf("source is a directory; use --recursive to upload directories")
				}
				if dryRun {
					fmt.Printf("Dry run: would upload directory %s to %s\n", source, dest)
					return nil
				}

				client, cleanup, err := createClient()
				if err != nil {
					return err
				}
				defer cleanup()

				ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutUpload)
				defer cancel()

				fmt.Println("Uploading directory...")
				return uploadDirectory(ctx, client, source, dest, namespace, contentType)
			}

			// Upload single file
			if dryRun {
				fmt.Printf("Dry run: would upload file %s to %s\n", source, dest)
				return nil
			}

			client, cleanup, err := createClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutUpload)
			defer cancel()

			fmt.Println("Uploading file...")
			return uploadFile(ctx, client, source, dest, namespace, contentType)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Namespace")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Upload directories recursively")
	cmd.Flags().BoolVar(&createDirs, "create-dirs", true, "Create parent directories")
	cmd.Flags().StringVar(&contentType, "content-type", "", "Content type")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be uploaded without uploading")

	return cmd
}

// newFilesDeleteCmd creates the delete command.
func newFilesDeleteCmd() *cobra.Command {
	var (
		namespace string
		recursive bool
		force     bool
		dryRun    bool
	)

	cmd := &cobra.Command{
		Use:   "delete <path>",
		Short: "Delete a file from the distribution system",
		Long: `Delete a file or directory from the distribution system.

Examples:
  kscore-files delete /packages/old-nginx.rpm
  kscore-files delete /temp/ --recursive
  kscore-files delete /configs/deprecated.yaml --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			if dryRun {
				fmt.Printf("Dry run: would delete %s\n", path)
				return nil
			}

			if !force {
				fmt.Printf("Delete %s? [y/N]: ", path)
				var confirm string
				fmt.Scanln(&confirm)
				if !strings.EqualFold(confirm, "y") && !strings.EqualFold(confirm, "yes") {
					fmt.Println("Cancelled")
					return nil
				}
			}

			client, cleanup, err := createClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutShort)
			defer cancel()

			opts := &files.DeleteFileOptions{
				Namespace: namespace,
				Recursive: recursive,
			}

			if err := client.DeleteFile(ctx, path, opts); err != nil {
				return fmt.Errorf("failed to delete file: %w", err)
			}

			fmt.Printf("Deleted %s\n", path)
			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Namespace")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Delete directories recursively")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Don't prompt for confirmation")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be deleted without deleting")

	return cmd
}

// newFilesInfoCmd creates the info command.
func newFilesInfoCmd() *cobra.Command {
	var (
		namespace string
		outputFmt string
	)

	cmd := &cobra.Command{
		Use:   "info <path>",
		Short: "Get file information",
		Long: `Get detailed information about a file.

Examples:
  kscore-files info /packages/nginx-1.20.rpm
  kscore-files info /configs/app.yaml --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			client, cleanup, err := createClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutShort)
			defer cancel()

			info, err := client.GetFileInfo(ctx, path, &files.GetFileInfoOptions{
				Namespace: namespace,
			})
			if err != nil {
				return fmt.Errorf("failed to get file info: %w", err)
			}

			format, err := output.ParseFormat(outputFmt)
			if err != nil {
				return err
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, info)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, info)
			case output.FormatTable:
				table := buildKeyValueTable([][2]string{
					{"PATH", info.Path},
					{"SIZE", fmt.Sprintf("%s (%d bytes)", formatSize(info.Size), info.Size)},
					{"MODIFIED", info.ModifiedTime.Format(time.RFC3339)},
					{"CHECKSUM", info.Checksum},
					{"CONTENT-TYPE", info.ContentType},
					{"VERSION", info.Version},
					{"TAGS", formatTags(info.Tags)},
				})
				return output.WriteTable(os.Stdout, table)
			case output.FormatText:
				fmt.Printf("Path:        %s\n", info.Path)
				fmt.Printf("Size:        %s (%d bytes)\n", formatSize(info.Size), info.Size)
				fmt.Printf("Modified:    %s\n", info.ModifiedTime.Format(time.RFC3339))
				fmt.Printf("Checksum:    %s\n", info.Checksum)
				if info.ContentType != "" {
					fmt.Printf("Content-Type: %s\n", info.ContentType)
				}
				if info.Version != "" {
					fmt.Printf("Version:     %s\n", info.Version)
				}
				if len(info.Tags) > 0 {
					fmt.Println("Tags:")
					for k, v := range info.Tags {
						fmt.Printf("  %s: %s\n", k, v)
					}
				}
				return nil
			default:
				return fmt.Errorf("unsupported output format: %s", outputFmt)
			}
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Namespace")
	cmd.Flags().StringVarP(&outputFmt, "output", "o", "text", "Output format (text, json, yaml, table)")

	return cmd
}

// newFilesSyncCmd creates the sync command.
func newFilesSyncCmd() *cobra.Command {
	var (
		namespace   string
		deleteExtra bool
		dryRun      bool
		checksum    bool
	)

	cmd := &cobra.Command{
		Use:   "sync <source> <destination>",
		Short: "Synchronize files between local and remote",
		Long: `Synchronize a local directory with a remote path.

Examples:
  kscore-files sync ./configs/ /configs/
  kscore-files sync /packages/ ./local-packages/ --delete
  kscore-files sync ./app/ /app/ --dry-run`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]
			dest := args[1]

			client, cleanup, err := createClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutSync)
			defer cancel()

			// Determine direction (upload or download)
			srcInfo, srcErr := os.Stat(source)
			isUpload := srcErr == nil && srcInfo.IsDir()

			if isUpload {
				return syncUpload(ctx, client, source, dest, namespace, deleteExtra, dryRun, checksum)
			}

			// Download sync
			return syncDownload(ctx, client, source, dest, namespace, deleteExtra, dryRun, checksum)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Namespace")
	cmd.Flags().BoolVar(&deleteExtra, "delete", false, "Delete extraneous files from destination")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done")
	cmd.Flags().BoolVarP(&checksum, "checksum", "c", false, "Skip files based on checksum")

	return cmd
}

// Helper functions

func createClient() (*files.Client, func(), error) {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to NATS (check --nats-url or server availability): %w", err)
	}

	client, err := files.NewClient(&files.ClientConfig{
		ClusterID: clusterID,
		AgentID:   instanceID,
	})
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("failed to create client: %w", err)
	}

	if err := client.Connect(nc); err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("failed to connect client: %w", err)
	}

	cleanup := func() {
		nc.Close()
	}

	return client, cleanup, nil
}

func formatSize(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/GB)
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/MB)
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/KB)
	default:
		return fmt.Sprintf("%d B", size)
	}
}

func uploadFile(ctx context.Context, client *files.Client, source, dest, namespace, contentType string) error {
	file, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	// Compute checksum
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("failed to compute checksum: %w", err)
	}
	checksum := "sha256:" + hex.EncodeToString(hash.Sum(nil))

	// Reset file for reading
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek file: %w", err)
	}

	opts := &files.PutFileOptions{
		Namespace:   namespace,
		ContentType: contentType,
		Checksum:    checksum,
	}

	if err := client.PutFile(ctx, dest, file, info.Size(), opts); err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}

	fmt.Printf("Uploaded %s -> %s (%s)\n", source, dest, formatSize(info.Size()))
	return nil
}

func uploadDirectory(ctx context.Context, client *files.Client, source, dest, namespace, contentType string) error {
	var uploaded int
	var totalSize int64

	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Calculate relative path
		relPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}

		// Build destination path
		destPath := filepath.Join(dest, relPath)
		destPath = filepath.ToSlash(destPath) // Normalize to forward slashes

		if err := uploadFile(ctx, client, path, destPath, namespace, contentType); err != nil {
			return err
		}

		uploaded++
		totalSize += info.Size()
		return nil
	})

	if err != nil {
		return err
	}

	fmt.Printf("\nUploaded %d files (%s total)\n", uploaded, formatSize(totalSize))
	return nil
}

func syncUpload(ctx context.Context, client *files.Client, source, dest, namespace string, deleteExtra, dryRun, useChecksum bool) error {
	// List local files
	localFiles := make(map[string]os.FileInfo)
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		localFiles[filepath.ToSlash(relPath)] = info
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk local directory: %w", err)
	}

	// List remote files
	result, err := client.ListFiles(ctx, dest, &files.ListFilesOptions{
		Namespace: namespace,
		Recursive: true,
	})
	if err != nil {
		return fmt.Errorf("failed to list remote files: %w", err)
	}

	remoteFiles := make(map[string]*files.FileEntry)
	for _, entry := range result.Entries {
		if !entry.IsDir {
			relPath := strings.TrimPrefix(entry.Path, dest)
			relPath = strings.TrimPrefix(relPath, "/")
			remoteFiles[relPath] = entry
		}
	}

	// Find files to upload
	var toUpload []string
	for path, localInfo := range localFiles {
		remote, exists := remoteFiles[path]
		if !exists {
			toUpload = append(toUpload, path)
			continue
		}

		// Check if needs update
		if useChecksum {
			// Compute local checksum
			localPath := filepath.Join(source, filepath.FromSlash(path))
			localChecksum, err := computeFileChecksum(localPath)
			if err != nil {
				return fmt.Errorf("failed to compute checksum for %s: %w", path, err)
			}
			if localChecksum != remote.Checksum {
				toUpload = append(toUpload, path)
			}
		} else if localInfo.ModTime().After(remote.ModTime) {
			toUpload = append(toUpload, path)
		}
	}

	// Find files to delete
	var toDelete []string
	if deleteExtra {
		for path := range remoteFiles {
			if _, exists := localFiles[path]; !exists {
				toDelete = append(toDelete, path)
			}
		}
	}

	// Execute sync
	if dryRun {
		fmt.Println("Dry run - would perform the following actions:")
		for _, path := range toUpload {
			fmt.Printf("  UPLOAD: %s\n", path)
		}
		for _, path := range toDelete {
			fmt.Printf("  DELETE: %s\n", path)
		}
		return nil
	}

	for _, path := range toUpload {
		localPath := filepath.Join(source, filepath.FromSlash(path))
		remotePath := filepath.ToSlash(filepath.Join(dest, path))
		if err := uploadFile(ctx, client, localPath, remotePath, namespace, ""); err != nil {
			return err
		}
	}

	for _, path := range toDelete {
		remotePath := filepath.ToSlash(filepath.Join(dest, path))
		if err := client.DeleteFile(ctx, remotePath, &files.DeleteFileOptions{Namespace: namespace}); err != nil {
			return fmt.Errorf("failed to delete %s: %w", remotePath, err)
		}
		fmt.Printf("Deleted %s\n", remotePath)
	}

	fmt.Printf("\nSync complete: %d uploaded, %d deleted\n", len(toUpload), len(toDelete))
	return nil
}

func syncDownload(ctx context.Context, client *files.Client, source, dest, namespace string, deleteExtra, dryRun, useChecksum bool) error {
	// List remote files
	result, err := client.ListFiles(ctx, source, &files.ListFilesOptions{
		Namespace: namespace,
		Recursive: true,
	})
	if err != nil {
		return fmt.Errorf("failed to list remote files: %w", err)
	}

	remoteFiles := make(map[string]*files.FileEntry)
	for _, entry := range result.Entries {
		if !entry.IsDir {
			relPath := strings.TrimPrefix(entry.Path, source)
			relPath = strings.TrimPrefix(relPath, "/")
			remoteFiles[relPath] = entry
		}
	}

	// List local files
	localFiles := make(map[string]os.FileInfo)
	if info, err := os.Stat(dest); err == nil && info.IsDir() {
		filepath.Walk(dest, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			relPath, err := filepath.Rel(dest, path)
			if err != nil {
				return err
			}
			localFiles[filepath.ToSlash(relPath)] = info
			return nil
		})
	}

	// Find files to download
	var toDownload []string
	for path, remote := range remoteFiles {
		local, exists := localFiles[path]
		if !exists {
			toDownload = append(toDownload, path)
			continue
		}

		// Check if needs update
		if useChecksum {
			localPath := filepath.Join(dest, filepath.FromSlash(path))
			localChecksum, err := computeFileChecksum(localPath)
			if err != nil {
				return fmt.Errorf("failed to compute checksum for %s: %w", path, err)
			}
			if localChecksum != remote.Checksum {
				toDownload = append(toDownload, path)
			}
		} else if remote.ModTime.After(local.ModTime()) {
			toDownload = append(toDownload, path)
		}
	}

	// Find files to delete
	var toDelete []string
	if deleteExtra {
		for path := range localFiles {
			if _, exists := remoteFiles[path]; !exists {
				toDelete = append(toDelete, path)
			}
		}
	}

	// Execute sync
	if dryRun {
		fmt.Println("Dry run - would perform the following actions:")
		for _, path := range toDownload {
			fmt.Printf("  DOWNLOAD: %s\n", path)
		}
		for _, path := range toDelete {
			fmt.Printf("  DELETE: %s\n", path)
		}
		return nil
	}

	for _, path := range toDownload {
		remotePath := filepath.ToSlash(filepath.Join(source, path))
		localPath := filepath.Join(dest, filepath.FromSlash(path))

		// Create parent directory
		//nolint:gosec // G301: destination directory needs to be accessible by users
		if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		// Download file - prepend namespace if specified
		getPath := remotePath
		if namespace != "" {
			getPath = namespace + "/" + remotePath
		}
		getResult, err := client.GetFile(ctx, getPath, &files.GetFileOptions{})
		if err != nil {
			return fmt.Errorf("failed to download %s: %w", remotePath, err)
		}

		// Copy to destination
		srcFile, err := os.Open(getResult.LocalPath)
		if err != nil {
			return fmt.Errorf("failed to open downloaded file: %w", err)
		}

		destFile, err := os.Create(localPath)
		if err != nil {
			srcFile.Close()
			return fmt.Errorf("failed to create local file: %w", err)
		}

		_, err = io.Copy(destFile, srcFile)
		srcFile.Close()
		destFile.Close()
		if err != nil {
			return fmt.Errorf("failed to copy file: %w", err)
		}

		fmt.Printf("Downloaded %s\n", path)
	}

	for _, path := range toDelete {
		localPath := filepath.Join(dest, filepath.FromSlash(path))
		if err := os.Remove(localPath); err != nil {
			return fmt.Errorf("failed to delete %s: %w", localPath, err)
		}
		fmt.Printf("Deleted %s\n", path)
	}

	fmt.Printf("\nSync complete: %d downloaded, %d deleted\n", len(toDownload), len(toDelete))
	return nil
}

func computeFileChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
