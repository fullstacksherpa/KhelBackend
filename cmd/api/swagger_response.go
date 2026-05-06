package main

import "khel/internal/domain/inventory"

type ErrorResponse struct {
	Error string `json:"error" example:"Bad request"`
}

type MessageResponse struct {
	Data MessageData `json:"data"`
}

type MessageData struct {
	Message string `json:"message" example:"inventory item deleted successfully"`
}

type InventoryItemResponse struct {
	Data inventory.InventoryItem `json:"data"`
}

type InventoryItemsResponse struct {
	Data []inventory.InventoryItem `json:"data"`
}

type ActiveGamesResponse struct {
	Data []ActiveGameResponseItem `json:"data"`
}

type GameDetailResponse struct {
	Data inventory.GameDetail `json:"data"`
}

type AddItemToGameResponse struct {
	Data AddItemToGameData `json:"data"`
}
