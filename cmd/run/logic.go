package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jo-hoe/ai-proxy/internal/wincred"
)

const (
	defaultImage     = "proxy:latest"
	defaultContainer = "proxy"
	defaultArchive   = "proxy-linux.amd64.tar.gz"
	defaultBinary    = "proxy"
	tokenSecretPath  = "/run/secrets/refresh-token"
)

// options holds all parsed CLI options for the run command.
type options struct {
	image     string
	container string
	proxyPort string
	mgmtPort  string
	tokenFile string
	build     bool
}

func run(args []string, store wincred.Store) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	opts := &options{}
	fs.StringVar(&opts.image, "image", defaultImage, "Docker image name")
	fs.StringVar(&opts.container, "container", defaultContainer, "Docker container name")
	fs.StringVar(&opts.proxyPort, "proxy-port", "", "proxy port (default: from config.yaml)")
	fs.StringVar(&opts.mgmtPort, "mgmt-port", "7656", "management API port")
	fs.StringVar(&opts.tokenFile, "token-file", os.Getenv("TOKEN_FILE"), "path to an existing token file")
	fs.BoolVar(&opts.build, "build", false, "force image rebuild")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if opts.proxyPort == "" {
		opts.proxyPort = fmt.Sprintf("%d", cfg.proxyPort)
	}

	dir, err := execDir()
	if err != nil {
		return err
	}

	if opts.build || !imageExists(opts.image) {
		if err := ensureBinary(dir); err != nil {
			return err
		}
		if err := dockerBuild(opts.image, dir); err != nil {
			return err
		}
	}

	if containerExists(opts.container) {
		fmt.Printf("==> Stopping existing container %s...\n", opts.container)
		if err := dockerRm(opts.container); err != nil {
			return err
		}
	}

	tokenPath, cleanup, err := resolveToken(opts.tokenFile, dir, store)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}

	winPath, err := toWindowsPath(tokenPath)
	if err != nil {
		return fmt.Errorf("resolve token path for Docker: %w", err)
	}

	fmt.Printf("==> Starting container on proxy port %s, management port %s...\n",
		opts.proxyPort, opts.mgmtPort)
	if err := dockerRun(opts, winPath); err != nil {
		return err
	}

	fmt.Println("==> Waiting for proxy to start...")
	time.Sleep(4 * time.Second)
	dockerLogs(opts.container)

	fmt.Printf("\nProxy listening at http://localhost:%s/openai/v1/\n", opts.proxyPort)
	fmt.Printf("Management API at http://localhost:%s/\n", opts.mgmtPort)
	fmt.Println("  POST /token   (form field: token=<refresh_token>)")
	fmt.Println("  GET  /status")
	fmt.Printf("Stop with: docker rm -f %s\n", opts.container)
	return nil
}

// resolveToken returns the path to a token file, obtaining one from the
// Credential Manager when tokenFile is empty. The cleanup func removes any
// temporary file created.
func resolveToken(tokenFile, dir string, store wincred.Store) (path string, cleanup func(), err error) {
	if tokenFile != "" {
		info, statErr := os.Stat(tokenFile)
		if statErr != nil || info.Size() == 0 {
			return "", nil, fmt.Errorf("token file is empty or missing: %s", tokenFile)
		}
		return tokenFile, nil, nil
	}

	fmt.Println("==> Extracting refresh token from Windows Credential Manager...")
	tmp := filepath.Join(dir, ".token-run")
	creds, err := store.FindByPrefix("proxy-cli:http")
	if err != nil {
		return "", nil, fmt.Errorf("credential lookup: %w", err)
	}
	creds = wincred.Filter(creds, []string{"proxy-api-key"})
	if len(creds) == 0 {
		return "", nil, errors.New("no proxy credentials found in Credential Manager — run SSO login first")
	}

	token := creds[0].Token
	if strings.TrimSpace(token) == "" {
		return "", nil, fmt.Errorf("credential %q has an empty token", creds[0].Target)
	}
	if err := os.WriteFile(tmp, []byte(token), 0600); err != nil {
		return "", nil, fmt.Errorf("write temp token: %w", err)
	}
	return tmp, func() { os.Remove(tmp) }, nil
}

// ensureBinary extracts the proxy binary from the archive if not already present.
func ensureBinary(dir string) error {
	binPath := filepath.Join(dir, defaultBinary)
	if _, err := os.Stat(binPath); err == nil {
		return nil
	}
	archivePath := filepath.Join(dir, defaultArchive)
	fmt.Printf("==> Extracting proxy binary from %s...\n", defaultArchive)
	return extractFromTar(archivePath, defaultBinary, binPath)
}

// extractFromTar extracts a single named file from a .tar.gz archive.
func extractFromTar(archivePath, entryName, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive %s: %w", archivePath, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("decompress archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}
		if filepath.Base(hdr.Name) != entryName {
			continue
		}
		out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
		if err != nil {
			return fmt.Errorf("create binary: %w", err)
		}
		_, copyErr := io.Copy(out, tr)
		closeErr := out.Close()
		if copyErr != nil {
			return fmt.Errorf("write binary: %w", copyErr)
		}
		return closeErr
	}
	return fmt.Errorf("entry %q not found in archive %s", entryName, archivePath)
}

func dockerBuild(image, contextDir string) error {
	fmt.Printf("==> Building %s...\n", image)
	return runCmd("docker", "build", "-t", image, contextDir)
}

func dockerRm(container string) error {
	return runCmd("docker", "rm", "-f", container)
}

func dockerRun(opts *options, tokenWinPath string) error {
	return runCmd("docker", "run", "-d",
		"--name", opts.container,
		"-p", opts.proxyPort+":"+opts.proxyPort,
		"-p", opts.mgmtPort+":"+opts.mgmtPort,
		"-e", "PROXY_PORT="+opts.proxyPort,
		"-e", "MGMT_PORT="+opts.mgmtPort,
		"-v", tokenWinPath+":"+tokenSecretPath+":ro",
		"--restart", "unless-stopped",
		opts.image,
	)
}

func dockerLogs(container string) {
	out, err := exec.Command("docker", "logs", "--tail", "8", container).CombinedOutput()
	if err == nil {
		fmt.Print(string(out))
	}
}

func imageExists(image string) bool {
	err := exec.Command("docker", "image", "inspect", image).Run()
	return err == nil
}

func containerExists(container string) bool {
	err := exec.Command("docker", "inspect", container).Run()
	return err == nil
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// execDir returns the directory containing the running executable.
func execDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	return filepath.Dir(exe), nil
}

// configData holds the subset of config.yaml values needed by run.
type configData struct {
	proxyPort int
}

func loadConfig() (*configData, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable: %w", err)
	}
	configPath := filepath.Join(filepath.Dir(exe), "config.yaml")
	cfg, err := parseConfigFile(configPath)
	if err != nil {
		return &configData{proxyPort: 7655}, nil
	}
	return &configData{proxyPort: cfg.Proxy.Port}, nil
}
