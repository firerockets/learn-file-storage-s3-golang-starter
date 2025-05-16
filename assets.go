package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
)

type videoMeta struct {
	Streams []struct {
		Width  int `json:"width,omitempty"`
		Height int `json:"height,omitempty"`
	} `json:"streams"`
}

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Run()

	var meta videoMeta

	err := json.Unmarshal(out.Bytes(), &meta)

	if err != nil {
		return "", err
	}

	if len(meta.Streams) == 0 {
		return "", fmt.Errorf("streams array is empty")
	}

	width := meta.Streams[0].Width
	height := meta.Streams[0].Height

	gcd := getGCD(width, height)

	aspectRatio := fmt.Sprintf("%v:%v", width/gcd, height/gcd)

	switch aspectRatio {
	case "16:9", "9:16":
		return aspectRatio, nil
	default:
		return "other", nil
	}
}

func processVideoForFastStart(filePath string) (string, error) {
	destFilePath := filePath + ".processing"

	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", destFilePath)
	var out strings.Builder
	cmd.Stdout = &out
	err := cmd.Run()

	if err != nil {
		return "", err
	}

	return destFilePath, nil
}

func getGCD(l, r int) int {
	gcd := r

	remainder := math.MaxInt

	for remainder > 0 {
		remainder := r % l
		if remainder == 0 {
			gcd = l
			break
		}
		r = l
		l = remainder
	}

	return gcd
}
