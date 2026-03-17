package main

type AlbumResponseDto struct {
	AlbumName  string `json:"albumName"`
	Id         string `json:"id"`
	AssetCount int64  `json:"assetCount"`
}

type CreateAlbumRequest struct {
	AlbumName string   `json:"albumName"`
	AssetIDs  []string `json:"assetIds"`
}

type CreateAlbumDto struct {
	AlbumName string `json:"albumName"`
	ID        string `json:"id"`
}

type AddAssetsToAlbumRequest struct {
	AssetIds []string `json:"assetIds"`
	AlbumIds []string `json:"albumIds"`
}

type AddAssetsToAlbumResponse struct {
	Error   string `json:"error"`
	Success bool   `json:"success"`
}
