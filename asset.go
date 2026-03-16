package main

import (
	"time"
)

type AssetMediaRequest struct {
	DeviceAssetId  string    `json:"deviceAssetId"`
	DeviceId       string    `json:"deviceId"`
	FileCreatedAt  time.Time `json:"fileCreatedAt"`
	FileModifiedAt time.Time `json:"fileModifiedAt"`
	Filename       string    `json:"filename"`
}

type AssetMediaResponseDto struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}
