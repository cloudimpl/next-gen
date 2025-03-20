package main

import (
	"flag"
	"fmt"
	"github.com/cloudimpl/next-gen/lib"
	"github.com/fsnotify/fsnotify"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
)

func watch(appPath string, onChange func()) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Close()

	// Handle OS signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Received termination signal, shutting down watcher...")
		watcher.Close()
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if event.Op&fsnotify.Create == fsnotify.Create {
					info, err := os.Stat(event.Name)
					if err == nil && info.IsDir() {
						log.Printf("New directory detected: %s, adding to watcher", event.Name)
						if err := watcher.Add(event.Name); err != nil {
							log.Printf("Failed to watch new directory: %s, error: %v", event.Name, err)
						}
					}
				}

				if event.Op&fsnotify.Write == fsnotify.Write {
					if lib.IsGoFile(event.Name) {
						if err := lib.CheckFileCompilable(event.Name); err == nil {
							log.Printf("Change detected in: %s, triggering onChange", event.Name)
							onChange()
						} else {
							log.Printf("File not compilable: %s, error: %v", event.Name, err)
						}
					}
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("Watcher error: %v", err)
			}
		}
	}()

	err = filepath.Walk(appPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("Error walking path: %s, error: %v", path, err)
			return err
		}
		if info.IsDir() {
			log.Printf("Adding directory to watcher: %s", path)
			return watcher.Add(path)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to walk path: %v", err)
	}

	<-done
}

func generate(appPath string) error {
	err := lib.GenerateServices(appPath, true)
	if err != nil {
		return fmt.Errorf("Error generating services: %s\n", err.Error())
	}

	return nil
}

func watchAndGenerate(appPath string) {
	// Ensure the directory exists
	if _, err := os.Stat(appPath); os.IsNotExist(err) {
		log.Fatalf("APP_PATH does not exist: %s", appPath)
	}

	servicesPath := filepath.Join(appPath, "services")
	log.Printf("Starting watcher on: %s", servicesPath)

	watch(servicesPath, func() {
		err := generate(appPath)
		if err != nil {
			log.Println(err.Error())
		}
	})
}

// isGoImportsAvailable checks if the `goimports` command is available
func isGoImportsAvailable() bool {
	_, err := exec.LookPath("goimports")
	return err == nil
}

// installGoImports installs the `goimports` tool using `go install`
func installGoImports() error {
	cmd := exec.Command("go", "install", "golang.org/x/tools/cmd/goimports@latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current working directory: %v", err)
	}

	var appPath string
	watch := flag.Bool("w", false, "watch for changes")
	flag.StringVar(&appPath, "f", cwd, "app path")
	flag.Parse()

	// Check if `goimports` is installed
	if !isGoImportsAvailable() {
		log.Println("goimports is not installed. Installing now...")

		// Attempt to install `goimports`
		err := installGoImports()
		if err != nil {
			log.Fatalf("Failed to install goimports: %v. Please install it manually by running:\n\tgo install golang.org/x/tools/cmd/goimports@latest", err)
		}

		log.Println("goimports successfully installed.")
	}

	if *watch {
		watchAndGenerate(appPath)
	} else {
		err := generate(appPath)
		if err != nil {
			log.Fatalf(err.Error())
		}
	}
}
