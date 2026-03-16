package main

import (
	"bytes"
	"context"
	"encoding/json"
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
	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(jsonData))
	if err != nil {
		return result, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	respBuff, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(respBuff, &result); err != nil {
		return result, err
	}
	return result, nil
}

func PostAsset(url *url.URL, request AssetMediaRequest, reader io.Reader, apiKey string) (result AssetMediaResponseDto, err error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("deviceAssetId", request.DeviceAssetId)
	_ = writer.WriteField("deviceId", request.DeviceId)
	_ = writer.WriteField("fileCreatedAt", request.FileCreatedAt.Format(time.RFC3339))
	_ = writer.WriteField("fileModifiedAt", request.FileModifiedAt.Format(time.RFC3339))
	_ = writer.WriteField("filename", request.Filename)

	part, err := writer.CreateFormFile("assetData", request.Filename) // "file_param_name" is the field name on the server side
	if err != nil {
		slog.Error("Error creating form file:", slog.String("error", err.Error()))
		return
	}

	io.Copy(part, reader)
	_ = writer.Close()

	req, err := http.NewRequest("POST", url.String(), &body)
	if err != nil {
		return
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	respBuff, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(respBuff, &result); err != nil {
		return result, err
	}
	return result, nil
}

func Get[R any](url *url.URL, apiKey string) (result R, err error) {
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("x-api-key", apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	respBuff, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(respBuff, &result); err != nil {
		return result, err
	}
	return result, nil
}

func Put[R any](url *url.URL, request interface{}, apiKey string) (result R, err error) {
	body, err := json.Marshal(request)
	if err != nil {
		return result, err
	}
	req, err := http.NewRequest("PUT", url.String(), bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	respBuff, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(respBuff, &result); err != nil {
		return result, err
	}
	return result, nil
}

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	err := godotenv.Load()
	if err == nil {
		slog.Info("use settings from env file.")
	}

	// Retrieve variables using os.Getenv
	immichURL := os.Getenv("IMMICH_URL")
	immichAPIKey := os.Getenv("IMMICH_API_KEY")

	slog.Info("Immich instance", slog.String("url", immichURL), slog.String("api_key", immichAPIKey))

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
		albumName, err := filepath.Rel(inputDir, path)
		if err != nil {
			slog.Error("failed to get album name", slog.String("error", err.Error()))
			return nil
		}
		slog.Debug("album name", slog.String("album_name", albumName))

		if slices.ContainsFunc(albums, func(a AlbumResponseDto) bool {
			return a.AlbumName == albumName
		}) {
			slog.Info("album already exists. skipping.", slog.String("album_name", albumName))
			return nil
		}

		creatingAlbum := CreateAlbumRequest{
			AlbumName: albumName,
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

			if !slices.Contains(mediaExtensions, filepath.Ext(path)) {
				slog.Debug("skipping non-media file", slog.String("path", path))
				return nil
			}

			info, err := d.Info()
			if err != nil {
				return err
			}

			file, err := fsys.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			assetFileName := filepath.Join(albumName, path)

			h := fnv.New64()
			h.Write([]byte(assetFileName))
			deviceAssetId := h.Sum64()

			var request = AssetMediaRequest{
				DeviceAssetId:  strconv.FormatUint(deviceAssetId, 10),
				DeviceId:       "WEB",
				FileCreatedAt:  info.ModTime(),
				FileModifiedAt: info.ModTime(),
				Filename:       path,
			}

			asset, err := PostAsset(url.JoinPath("/api/assets"), request, file, immichAPIKey)
			if err != nil {
				return err
			}

			assetIds = append(assetIds, asset.ID)

			return nil
		})

		if err != nil {
			return err
		}

		addAssetResult, err := Put[AddAssetsToAlbumResponse](
			url.JoinPath("/api/albums/assets"), AddAssetsToAlbumRequest{
				AssetIds: assetIds,
				AlbumIds: []string{createdAlbum.ID},
			}, immichAPIKey)
		if err != nil {
			return err
		}

		slog.Info("add assets to album", slog.Any("result", addAssetResult))

		return nil
	})

	if err != nil {
		slog.Error("error creating albums", err)
	}
}
