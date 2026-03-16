package main

type AlbumResponseDto struct {
	AlbumName string `json:"albumName"`
	Id        string `json:"id"`
}

type CreateAlbumRequest struct {
	AlbumName string `json:"albumName"`
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
