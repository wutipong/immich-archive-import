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
