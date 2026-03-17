package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
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

	"github.com/lmittmann/tint"
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

var archiveExtensions = []string{
	".zip",
	".7z",
	".rar",
}

func Post[R any](url *url.URL, data any, apiKey string) (result R, err error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return result, err
	}

	body := bytes.NewBuffer(jsonData)

	return DoRequestWithResult[R]("POST", url, body, "application/json", apiKey)
}

func DoRequestWithResult[R any](
	method string,
	url *url.URL,
	body io.Reader,
	contentType string,
	apiKey string,
) (result R, err error) {
	resp, err := DoRequest(method, url, body, contentType, apiKey)
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

func DoRequest(
	method string,
	url *url.URL,
	body io.Reader,
	contentType string,
	apiKey string,
) (resp *http.Response, err error) {
	req, err := http.NewRequest(method, url.String(), body)
	if err != nil {
		return
	}
	req.Header.Set("x-api-key", apiKey)

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return http.DefaultClient.Do(req)
}

func Get[R any](url *url.URL, apiKey string) (result R, err error) {
	return DoRequestWithResult[R]("GET", url, nil, "", apiKey)
}

func Put[R any](url *url.URL, request any, apiKey string) (result R, err error) {
	body, err := json.Marshal(request)
	if err != nil {
		return result, err
	}

	return DoRequestWithResult[R]("PUT", url, bytes.NewReader(body), "application/json", apiKey)
}

func PostAsset(archivePath string, path string, reader io.Reader, modDate time.Time, url *url.URL, apiKey string) (result AssetMediaResponseDto, err error) {
	assetFileName := filepath.Join(archivePath, path)

	h := fnv.New64()
	h.Write([]byte(assetFileName))
	deviceAssetId := h.Sum64()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("deviceAssetId", strconv.FormatUint(deviceAssetId, 10))
	_ = writer.WriteField("deviceId", "WEB")
	_ = writer.WriteField("fileCreatedAt", modDate.Format(time.RFC3339))
	_ = writer.WriteField("fileModifiedAt", modDate.Format(time.RFC3339))
	_ = writer.WriteField("filename", path)

	part, err := writer.CreateFormFile("assetData", path)
	if err != nil {
		slog.Error("Error creating form file:", slog.String("error", err.Error()))
		return
	}

	_, err = io.Copy(part, reader)
	if err != nil {
		slog.Error("Error reading data:", slog.String("error", err.Error()))
		return
	}
	_ = writer.Close()

	return DoRequestWithResult[AssetMediaResponseDto]("POST", url.JoinPath("/api/assets"), &body, writer.FormDataContentType(), apiKey)
}

func DeleteAlbum(id string, url *url.URL, apiKey string) error {
	resp, err := DoRequest("DELETE", url.JoinPath("api", "albums", id), nil, "", apiKey)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 300 {
		return fmt.Errorf("request failed with status code %d: %s", resp.StatusCode, resp.Status)
	}

	return nil
}

func main() {
	// Set global logger with custom options
	slog.SetDefault(slog.New(
		tint.NewHandler(os.Stdout, &tint.Options{
			Level:      slog.LevelDebug,
			TimeFormat: time.Kitchen,
		}),
	))

	profileFlag := flag.String("profile", "default", "config profile to use")
	inputDirFlag := flag.String("dir", "", "input directory containing archive files")
	deleteEmptyAlbumsFlag := flag.Bool("delete-empty-albums", false, "Delete any empty albums.")
	flag.Parse()

	if *inputDirFlag == "" {
		slog.Error("input directory is required")
		return
	}

	slog.Info("parameter", slog.String("profile", *profileFlag), slog.String("dir", *inputDirFlag))

	homeDir, err := os.UserHomeDir()
	if err != nil {
		slog.Error("failed to get user home directory", slog.String("error", err.Error()))
		return
	}

	configPath := filepath.Join(homeDir, ".immich-archive-import", "config.yaml")

	config, err := LoadConfig(*profileFlag, configPath)
	if err != nil {
		slog.Error(
			"failed to load config. please check your config file.",
			slog.String("config path", configPath),
			slog.String("error", err.Error()),
		)

		return
	}

	switch config.LogLevel {
	case "debug":
		slog.SetLogLoggerLevel(slog.LevelDebug)
	case "info":
		slog.SetLogLoggerLevel(slog.LevelInfo)
	case "error":
		slog.SetLogLoggerLevel(slog.LevelError)
	default:
		slog.SetLogLoggerLevel(slog.LevelInfo)
	}

	immichURL := config.ImmichURL
	immichAPIKey := config.ImmichAPIKey

	slog.Info("Immich instance",
		slog.String("url", immichURL),
		slog.String("api_key",
			strings.Repeat("*", len(immichAPIKey)),
		),
	)

	url, err := url.Parse(immichURL)
	if err != nil {
		slog.Error("invalid immich url", slog.String("url", immichURL))
		return
	}

	inputDir := *inputDirFlag

	if *deleteEmptyAlbumsFlag {
		slog.Info("deleting empty albums")
		err := DeleteEmptyAlbums(url, immichAPIKey)
		if err != nil {
			slog.Error("failed to delete empty albums", slog.String("error", err.Error()))
			return
		}
	}

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

		if !slices.Contains(archiveExtensions, filepath.Ext(path)) {
			slog.Info("skipping non-archive file", slog.String("path", path))
			return nil
		}

		archiveFilePath, err := filepath.Rel(inputDir, path)
		if err != nil {
			return fmt.Errorf("failed to get album name: %w", err)
		}

		archiveFilePath = strings.ToValidUTF8(archiveFilePath, "-")
		slog.Debug("album name", slog.String("album_name", archiveFilePath))

		if slices.ContainsFunc(albums, func(a AlbumResponseDto) bool {
			return a.AlbumName == archiveFilePath
		}) {
			slog.Info("album already exists. skipping.", slog.String("album_name", archiveFilePath))
			return nil
		}

		ctx := context.Background()

		archiveFile, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to create virtual file system: %w", err)
		}
		defer archiveFile.Close()

		assetIds := make([]string, 0)

		err = WalkArchive(ctx, path, archiveFile, func(ctx context.Context, f archives.FileInfo) error {
			slog.Info("processing file", slog.String("archive", archiveFilePath), slog.String("entry", f.NameInArchive))

			file, err := f.Open()
			if err != nil {
				slog.Error("failed to open file in archive", slog.String("archive", archiveFilePath), slog.String("entry", f.NameInArchive), slog.String("error", err.Error()))
				return err
			}

			defer file.Close()

			asset, err := PostAsset(archiveFilePath, f.NameInArchive, file, f.ModTime(), url, immichAPIKey)
			if err != nil {
				slog.Error("failed to upload asset", slog.String("archive", archiveFilePath), slog.String("path", path), slog.String("error", err.Error()))
				return err
			}

			slog.Info("uploaded asset", slog.String("archive", archiveFilePath), slog.String("path", path), slog.Any("asset_response", asset))

			assetIds = append(assetIds, asset.ID)

			return nil
		})

		if err != nil {
			slog.Error("failed to process archive", slog.String("archive", archiveFilePath), slog.String("error", err.Error()))
			return err
		}

		slog.Info("creating album", slog.String("name", archiveFilePath))

		createdAlbum, err := Post[CreateAlbumDto](url.JoinPath("/api/albums"), CreateAlbumRequest{
			AlbumName: archiveFilePath,
			AssetIDs:  assetIds,
		}, immichAPIKey)
		if err != nil {
			return fmt.Errorf("failed to create album: %w", err)
		}

		slog.Info("created album", slog.Any("album", createdAlbum))

		return nil
	})

	if err != nil {
		slog.Error("error creating albums", slog.String("err", err.Error()))
	}
}

func WalkArchive(ctx context.Context, archivePath string, archive *os.File, walkFn func(ctx context.Context, f archives.FileInfo) error) error {
	format, stream, err := archives.Identify(ctx, archivePath, archive)
	if err != nil {
		return err
	}

	extractor, ok := format.(archives.Extractor)
	if !ok {
		return fmt.Errorf("format does not support extraction")
	}

	err = extractor.Extract(ctx, stream, func(ctx context.Context, f archives.FileInfo) error {
		if f.IsDir() {
			return nil
		}

		if slices.Contains(mediaExtensions, filepath.Ext(f.NameInArchive)) {
			return walkFn(ctx, f)
		}

		return nil
	})

	return err
}

func DeleteEmptyAlbums(url *url.URL, immichAPIKey string) error {
	albums, err := Get[[]AlbumResponseDto](url.JoinPath("/api/albums"), immichAPIKey)
	if err != nil {
		return fmt.Errorf("failed to get albums: %w", err)
	}

	slog.Debug("albums", slog.Any("albums", albums))
	for _, album := range albums {
		if album.AssetCount != 0 {
			continue
		}

		slog.Debug("Deleting album", slog.String("name", album.AlbumName), slog.String("id", album.Id))
		err = DeleteAlbum(album.Id, url, immichAPIKey)
		if err != nil {
			return fmt.Errorf("failed to delete album '%s': %w", album.AlbumName, err)
		}
	}
	return nil
}
