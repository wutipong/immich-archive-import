package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/mholt/archives"
)

var mediaExtensions = []string{
	// Image formats
	".avif",
	".bmp",
	".gif",
	".heic",
	".heif",
	".jp2",
	".jpeg",
	".jpg",
	".jpe",
	".insp",
	".jxl",
	".png",
	".psd",
	".raw",
	".rw2",
	".svg",
	".tif",
	".tiff",
	".webp",
	// video format
	".3gp",
	".3gpp",
	".avi",
	".flv",
	".m4v",
	".mkv",
	".mts",
	".m2ts",
	".m2t",
	".mp4",
	".insv",
	".mpg",
	".mpe",
	".mpeg",
	".mov",
	".webm",
	".wmv",
}

func Post[R any](url *url.URL, data interface{}, apiKey string) (result R, err error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return result, err
	}

	body := bytes.NewBuffer(jsonData)

	return DoRequest[R]("POST", url, body, "application/json", apiKey)
}

func DoRequest[R any](method string, url *url.URL, body io.Reader, contentType string, apiKey string) (result R, err error) {
	req, err := http.NewRequest(method, url.String(), body)
	if err != nil {
		return
	}
	req.Header.Set("x-api-key", apiKey)

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	respBuff, err := io.ReadAll(resp.Body)

	if err != nil {
		return
	}

	if resp.StatusCode >= 300 {
		err = fmt.Errorf("request failed with status code %d: %s", resp.StatusCode, string(respBuff))
		return
	}

	err = json.Unmarshal(respBuff, &result)
	if err != nil {
		return
	}
	return
}

func Get[R any](url *url.URL, apiKey string) (result R, err error) {
	return DoRequest[R]("GET", url, nil, "", apiKey)
}

func Put[R any](url *url.URL, request interface{}, apiKey string) (result R, err error) {
	body, err := json.Marshal(request)
	if err != nil {
		return result, err
	}

	return DoRequest[R]("PUT", url, bytes.NewReader(body), "application/json", apiKey)
}

func PostAsset(archivePath string, fsys fs.FS, path string, d fs.DirEntry, url *url.URL, apiKey string) (result AssetMediaResponseDto, err error) {
	info, err := d.Info()
	if err != nil {
		return
	}
	file, err := fsys.Open(path)
	if err != nil {
		slog.Error("failed to open file", slog.String("path", path), slog.String("error", err.Error()))
		return
	}
	defer file.Close()

	assetFileName := filepath.Join(archivePath, path)

	h := fnv.New64()
	h.Write([]byte(assetFileName))
	deviceAssetId := h.Sum64()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("deviceAssetId", strconv.FormatUint(deviceAssetId, 10))
	_ = writer.WriteField("deviceId", "WEB")
	_ = writer.WriteField("fileCreatedAt", info.ModTime().Format(time.RFC3339))
	_ = writer.WriteField("fileModifiedAt", info.ModTime().Format(time.RFC3339))
	_ = writer.WriteField("filename", path)

	part, err := writer.CreateFormFile("assetData", path)
	if err != nil {
		slog.Error("Error creating form file:", slog.String("error", err.Error()))
		return
	}

	_, err = io.Copy(part, file)
	if err != nil {
		slog.Error("Error copying file:", slog.String("error", err.Error()))
		return
	}
	_ = writer.Close()

	return DoRequest[AssetMediaResponseDto]("POST", url.JoinPath("/api/assets"), &body, writer.FormDataContentType(), apiKey)
}

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	err := godotenv.Load()
	if err == nil {
		slog.Info("use settings from env file.")
	}

	immichURL := os.Getenv("IMMICH_URL")
	immichAPIKey := os.Getenv("IMMICH_API_KEY")

	slog.Info("Immich instance", slog.String("url", immichURL), slog.String("api_key", strings.Repeat("*", len(immichAPIKey))))

	url, err := url.Parse(immichURL)
	if err != nil {
		slog.Error("invalid immich url", slog.String("url", immichURL))
		return
	}

	inputDir := os.Getenv("INPUT_DIR")
	slog.Info("input dir", slog.String("dir", inputDir))

	albums, err := Get[[]AlbumResponseDto](url.JoinPath("/api/albums"), immichAPIKey)
	if err != nil {
		slog.Error("failed to get albums", slog.String("error", err.Error()))
		return
	}
	slog.Info("albums", slog.Any("albums", albums))

	err = filepath.WalkDir(inputDir, func(path string, d os.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}
		archivePath, err := filepath.Rel(inputDir, path)
		if err != nil {
			slog.Error("failed to get album name", slog.String("error", err.Error()))
			return nil
		}
		slog.Debug("album name", slog.String("album_name", archivePath))

		if slices.ContainsFunc(albums, func(a AlbumResponseDto) bool {
			return a.AlbumName == archivePath
		}) {
			slog.Info("album already exists. skipping.", slog.String("album_name", archivePath))
			return nil
		}

		creatingAlbum := CreateAlbumRequest{
			AlbumName: archivePath,
		}

		createdAlbum, err := Post[CreateAlbumDto](url.JoinPath("/api/albums"), creatingAlbum, immichAPIKey)
		if err != nil {
			slog.Error("failed to create album", slog.String("error", err.Error()))
			return err
		}

		slog.Info("created album", slog.Any("album", createdAlbum))

		ctx := context.Background()
		assetIds := make([]string, 0)
		fsys, err := archives.FileSystem(ctx, path, nil)
		if err != nil {
			slog.Error("failed to create virtual file system", slog.String("error", err.Error()))
			return err
		}

		err = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				return nil
			}

			slog.Info("processing file", slog.String("archive", archivePath), slog.String("path", path))

			if !slices.Contains(mediaExtensions, filepath.Ext(path)) {
				slog.Info("skipping non-media file", slog.String("archive", archivePath), slog.String("path", path))
				return nil
			}

			asset, err := PostAsset(archivePath, fsys, path, d, url, immichAPIKey)
			if err != nil {
				slog.Info("failed to upload asset", slog.String("archive", archivePath), slog.String("path", path), slog.String("error", err.Error()))
				return err
			}

			slog.Info("uploaded asset", slog.String("archive", archivePath), slog.String("path", path), slog.Any("asset_response", asset))

			assetIds = append(assetIds, asset.ID)

			return nil
		})

		if err != nil {
			return err
		}

		slog.Info("Add assets to album", slog.String("album_name", archivePath), slog.Int("asset_count", len(assetIds)))

		result, err := Put[AddAssetsToAlbumResponse](
			url.JoinPath("/api/albums/assets"), AddAssetsToAlbumRequest{
				AssetIds: assetIds,
				AlbumIds: []string{createdAlbum.ID},
			}, immichAPIKey)
		if err != nil {
			return err
		}

		if !result.Success {
			slog.Error("failed to add assets to album", slog.Any("error", result.Error))
			return fmt.Errorf(result.Error)
		}

		return nil
	})

	if err != nil {
		slog.Error("error creating albums", err)
	}
}
