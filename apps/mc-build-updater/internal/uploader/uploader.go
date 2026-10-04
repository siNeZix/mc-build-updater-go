package uploader

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Config struct {
	Host           string
	Port           string
	User           string
	PrivateKeyPath string
	LocalModsPath  string
	RemoteModsPath string
	Workers        int
}

type fileInfo struct {
	name string
	size int64
}

func ConfigFromEnvironment() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve home directory: %w", err)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return Config{}, fmt.Errorf("resolve working directory: %w", err)
	}

	configuration := Config{
		Host:           getEnv("MC_BU_SFTP_HOST", "mc.sinezix.ru"),
		Port:           getEnv("MC_BU_SFTP_PORT", "22"),
		User:           getEnv("MC_BU_SFTP_USER", "root"),
		PrivateKeyPath: getEnv("MC_BU_SFTP_KEY_PATH", filepath.Join(home, ".ssh", "id_rsa")),
		LocalModsPath:  getEnv("MC_BU_MODS_PATH", filepath.Join(workingDirectory, "mods")),
		RemoteModsPath: getEnv("MC_BU_SFTP_REMOTE_MODS_PATH", "/root/file-hosting/files/mods/"),
	}
	return configuration, nil
}

func UploadMissing(configuration Config) error {
	if configuration.Workers < 1 {
		configuration.Workers = 1
	}
	remoteFiles, err := listRemote(configuration)
	if err != nil {
		return err
	}
	localFiles, err := listLocal(configuration.LocalModsPath)
	if err != nil {
		return err
	}

	missing := make([]fileInfo, 0)
	for _, local := range localFiles {
		if remoteSize, exists := remoteFiles[local.name]; !exists || remoteSize != local.size {
			missing = append(missing, local)
		}
	}
	if len(missing) == 0 {
		fmt.Println("No new mods to upload.")
		return nil
	}

	fmt.Printf("Uploading %d mod(s) with %d worker(s)\n", len(missing), configuration.Workers)
	jobs := make(chan fileInfo)
	errors := make(chan error, len(missing))
	var workers sync.WaitGroup
	for index := 0; index < min(configuration.Workers, len(missing)); index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for file := range jobs {
				if err := uploadOne(configuration, file); err != nil {
					errors <- err
					continue
				}
				fmt.Printf("Uploaded %s\n", file.name)
			}
		}()
	}
	for _, file := range missing {
		jobs <- file
	}
	close(jobs)
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}

func listRemote(configuration Config) (map[string]int64, error) {
	client, closeClient, err := connect(configuration)
	if err != nil {
		return nil, err
	}
	defer closeClient()

	entries, err := client.ReadDir(configuration.RemoteModsPath)
	if err != nil {
		return nil, fmt.Errorf("list remote mods: %w", err)
	}
	files := make(map[string]int64, len(entries))
	for _, entry := range entries {
		if entry.Mode().IsRegular() {
			files[entry.Name()] = entry.Size()
		}
	}
	return files, nil
}

func listLocal(directory string) ([]fileInfo, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("list local mods: %w", err)
	}
	files := make([]fileInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		files = append(files, fileInfo{name: entry.Name(), size: info.Size()})
	}
	return files, nil
}

func uploadOne(configuration Config, file fileInfo) error {
	client, closeClient, err := connect(configuration)
	if err != nil {
		return err
	}
	defer closeClient()

	source, err := os.Open(filepath.Join(configuration.LocalModsPath, file.name))
	if err != nil {
		return fmt.Errorf("open %s: %w", file.name, err)
	}
	defer source.Close()

	destination, err := client.Create(remotePath(configuration.RemoteModsPath, file.name))
	if err != nil {
		return fmt.Errorf("create remote %s: %w", file.name, err)
	}
	if _, err := io.Copy(destination, source); err != nil {
		destination.Close()
		return fmt.Errorf("upload %s: %w", file.name, err)
	}
	if err := destination.Close(); err != nil {
		return fmt.Errorf("close remote %s: %w", file.name, err)
	}
	return nil
}

func connect(configuration Config) (*sftp.Client, func(), error) {
	privateKey, err := os.ReadFile(configuration.PrivateKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read SFTP private key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("parse SFTP private key: %w", err)
	}
	connection, err := ssh.Dial("tcp", net.JoinHostPort(configuration.Host, configuration.Port), &ssh.ClientConfig{
		User:            configuration.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Legacy tool had no host-key verification; configurable trust can be added without changing upload behavior.
	})
	if err != nil {
		return nil, nil, fmt.Errorf("connect to SFTP: %w", err)
	}
	client, err := sftp.NewClient(connection)
	if err != nil {
		connection.Close()
		return nil, nil, fmt.Errorf("create SFTP client: %w", err)
	}
	closeClient := func() {
		client.Close()
		connection.Close()
	}
	return client, closeClient, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func remotePath(directory, fileName string) string {
	return strings.TrimRight(directory, "/") + "/" + fileName
}

func min(first, second int) int {
	if first < second {
		return first
	}
	return second
}
