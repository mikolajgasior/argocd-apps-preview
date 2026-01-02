package files

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/keenbytes/argocd-apps-preview/pkg/logmsg"
)

func DownloadFile(url string) (string, error) {
	logmsg.Info(fmt.Sprintf("Downloading %s...", url))

	tmpDir, err := os.MkdirTemp("", "download-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("GET request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status: %s", resp.Status)
	}

	fileName := filepath.Base(resp.Request.URL.Path)
	if fileName == "" || fileName == "." || fileName == "/" {
		fileName = "downloaded-file"
	}
	destPath := filepath.Join(tmpDir, fileName)

	outFile, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("creating file: %w", err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return "", fmt.Errorf("writing to file: %w", err)
	}

	logmsg.Info(fmt.Sprintf("File saved to %s.", destPath))
	
	return destPath, nil
}
