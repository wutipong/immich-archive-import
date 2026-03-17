package main

import (
	"bytes"
	"encoding/json"
	"errors"
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

	"github.com/gen2brain/go-unarr"
	"github.com/saintfish/chardet"
	"golang.org/x/text/encoding/ianaindex"
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

func PostAsset(archivePath string,
	entryName string,
	archive *unarr.Archive,
	url *url.URL,
	apiKey string,
) (result AssetMediaResponseDto, err error) {

	assetFileName := filepath.Join(archivePath, entryName)
	modDate := archive.ModTime()

	h := fnv.New64()
	h.Write([]byte(assetFileName))
	deviceAssetId := h.Sum64()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("deviceAssetId", strconv.FormatUint(deviceAssetId, 10))
	_ = writer.WriteField("deviceId", "WEB")
	_ = writer.WriteField("fileCreatedAt", modDate.Format(time.RFC3339))
	_ = writer.WriteField("fileModifiedAt", modDate.Format(time.RFC3339))
	_ = writer.WriteField("filename", entryName)

	part, err := writer.CreateFormFile("assetData", entryName)
	if err != nil {
		slog.Error("Error creating form file:", slog.String("error", err.Error()))
		return
	}

	data, err := archive.ReadAll()
	if err != nil {
		slog.Error("Error reading from archive", slog.String("error", err.Error()))
	}

	_, err = part.Write(data)
	if err != nil {
		slog.Error("Error copying file:", slog.String("error", err.Error()))
		return
	}
	_ = writer.Close()

	return DoRequestWithResult[AssetMediaResponseDto](
		"POST",
		url.JoinPath("/api/assets"),
		&body,
		writer.FormDataContentType(),
		apiKey,
	)
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
		slog.Error("failed to load config. please check your config file.", slog.String("config path", configPath), slog.String("error", err.Error()))
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

	slog.Info("Immich instance", slog.String("url", immichURL), slog.String("api_key", strings.Repeat("*", len(immichAPIKey))))

	url, err := url.Parse(immichURL)
	if err != nil {
		slog.Error("invalid immich url", slog.String("url", immichURL))
		return
	}

	inputDir := *inputDirFlag

	albums, err := Get[[]AlbumResponseDto](url.JoinPath("/api/albums"), immichAPIKey)
	if err != nil {
		slog.Error("failed to get albums", slog.String("error", err.Error()))
		return
	}
	slog.Info("albums", slog.Any("albums", albums))

	deleteEmptyAlbums := *deleteEmptyAlbumsFlag

	if deleteEmptyAlbums {
		for _, album := range albums {
			if album.AssetCount != 0 {
				continue
			}

			slog.Info("Deleting album", slog.String("name", album.AlbumName), slog.String("id", album.Id))
			err = DeleteAlbum(album.Id, url, immichAPIKey)
			if err != nil {
				slog.Error("Error", slog.String("error", err.Error()))
			}
		}
	}

	err = filepath.WalkDir(inputDir, func(path string, d os.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}

		if !slices.Contains(archiveExtensions, filepath.Ext(path)) {
			slog.Info("skipping non-archive file", slog.String("path", path))
			return nil
		}

		archivePath, err := filepath.Rel(inputDir, path)
		if err != nil {
			slog.Error("failed to get album name", slog.String("error", err.Error()))
			return nil
		}

		archivePath = strings.ToValidUTF8(archivePath, "-")
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

		assetIds := make([]string, 0)

		archive, err := unarr.NewArchive(path)
		if err != nil {
			panic(err)
		}

		defer archive.Close()

		detector := chardet.NewTextDetector()

		for {
			err = archive.Entry()
			if err != nil {
				break
			}

			entryName := archive.Name()
			result, err := detector.DetectBest([]byte(archive.RawName()))
			if err != nil {
				slog.Warn("unable to determine charset.", slog.String("error", err.Error()))
			} else {
				slog.Info("detected charset.", slog.String("charset", result.Charset))

				encoding, err := ianaindex.IANA.Encoding(result.Charset)
				if err == nil {
					decoder := encoding.NewDecoder()
					b, err := decoder.Bytes([]byte(archive.RawName()))
					if err == nil {
						entryName = string(b)
					}
				}
			}

			slog.Info("processing file", slog.String("archive", archivePath), slog.String("path", entryName))

			if !slices.Contains(mediaExtensions, filepath.Ext(archive.Name())) {
				slog.Info("skipping non-media file", slog.String("archive", archivePath), slog.String("path", archive.Name()))
				return nil
			}

			asset, err := PostAsset(archivePath, entryName, archive, url, immichAPIKey)
			if err != nil {
				slog.Info("failed to upload asset", slog.String("archive", archivePath), slog.String("path", path), slog.String("error", err.Error()))
				return err
			}

			slog.Info("uploaded asset", slog.String("archive", archivePath), slog.String("path", path), slog.Any("asset_response", asset))

			assetIds = append(assetIds, asset.ID)
		}

		if err == io.EOF {
			err = nil
		}
		if err != nil {
			slog.Error("Error uploading assets", slog.String("error", err.Error()))
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
			return errors.New(result.Error)
		}

		return nil
	})

	if err != nil {
		slog.Error("error creating albums", slog.String("err", err.Error()))
	}
}
