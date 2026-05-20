package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"khel/internal/domain/inventory"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cloudinary/cloudinary-go/v2/api"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
	"github.com/go-chi/chi/v5"
)

func readIDParam(r *http.Request, name string) (int64, error) {
	idStr := chi.URLParam(r, name)
	return strconv.ParseInt(idStr, 10, 64)
}

func (app *application) uploadInventoryImageToCloudinary(
	file io.Reader,
	venueID int64,
	itemName string,
) (string, error) {
	env := os.Getenv("APP_ENV")

	folder := "testInventory"
	if env == "prod" || env == "production" {
		folder = "inventory"
	}

	safeItemName := app.createSafePublicID(itemName)
	publicID := fmt.Sprintf("venue_%d_%s_%d", venueID, safeItemName, time.Now().UnixNano())

	resp, err := app.cld.Upload.Upload(
		context.Background(),
		file,
		uploader.UploadParams{
			Folder:    folder,
			PublicID:  publicID,
			Overwrite: api.Bool(false),
			// good for small product images
			Transformation: "w_500,h_500,c_fill,q_auto,f_auto",
		},
	)

	if err != nil {
		return "", fmt.Errorf("cloudinary inventory upload failed: %w", err)
	}

	return resp.SecureURL, nil
}

// createInventoryItemHandler godoc
//
//	@Summary		Create venue inventory item
//	@Description	Creates a new inventory item for a venue with optional image upload.
//	@Tags			venue inventory
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			venueID			path		int						true	"Venue ID"
//	@Param			name			formData	string					true	"Inventory item name"
//	@Param			description		formData	string					false	"Inventory item description"
//	@Param			unit_price		formData	int						true	"Unit price"
//	@Param			stock_quantity	formData	int						false	"Stock quantity"
//	@Param			track_stock		formData	bool					false	"Track stock"
//	@Param			image			formData	file					false	"Inventory item image"
//	@Success		201				{object}	InventoryItemResponse	"Inventory item created"
//	@Failure		400				{object}	ErrorResponse			"Bad request"
//	@Failure		404				{object}	ErrorResponse			"Not found"
//	@Failure		500				{object}	ErrorResponse			"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/inventory [post]
func (app *application) createInventoryItemHandler(w http.ResponseWriter, r *http.Request) {
	venueID, err := readIDParam(r, "venueID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	err = r.ParseMultipartForm(10 << 20) // 10 MB
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	descriptionValue := strings.TrimSpace(r.FormValue("description"))
	unitPriceStr := r.FormValue("unit_price")
	stockQuantityStr := r.FormValue("stock_quantity")
	trackStockStr := r.FormValue("track_stock")

	unitPrice, err := strconv.Atoi(unitPriceStr)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid unit_price"))
		return
	}

	trackStock := trackStockStr == "true"

	var description *string
	if descriptionValue != "" {
		description = &descriptionValue
	}

	var stockQuantity *int
	if stockQuantityStr != "" {
		qty, err := strconv.Atoi(stockQuantityStr)
		if err != nil {
			app.badRequestResponse(w, r, fmt.Errorf("invalid stock_quantity"))
			return
		}
		stockQuantity = &qty
	}

	var imageURL *string

	file, fileHeader, err := r.FormFile("image")
	if err == nil {
		defer file.Close()

		if !app.isValidAdImageType(fileHeader.Header.Get("Content-Type")) {
			app.badRequestResponse(w, r, fmt.Errorf("invalid image type"))
			return
		}

		url, err := app.uploadInventoryImageToCloudinary(file, venueID, name)
		if err != nil {
			app.internalServerError(w, r, err)
			return
		}

		imageURL = &url
	}

	item := &inventory.InventoryItem{
		VenueID:       venueID,
		Name:          name,
		Description:   description,
		UnitPrice:     unitPrice,
		ImageURL:      imageURL,
		StockQuantity: stockQuantity,
		TrackStock:    trackStock,
	}

	err = app.store.Inventory.CreateInventoryItem(r.Context(), item)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusCreated, item)
}

// listInventoryItemsHandler godoc
//
//	@Summary		List venue inventory items
//	@Description	Returns all inventory items for a specific venue.
//	@Tags			venue inventory
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int						true	"Venue ID"
//	@Success		200		{object}	InventoryItemsResponse	"Venue inventory items"
//	@Failure		400		{object}	ErrorResponse			"Bad request"
//	@Failure		404		{object}	ErrorResponse			"Not found"
//	@Failure		500		{object}	ErrorResponse			"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/inventory [get]
func (app *application) listInventoryItemsHandler(w http.ResponseWriter, r *http.Request) {
	venueID, err := readIDParam(r, "venueID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	items, err := app.store.Inventory.ListInventoryItems(r.Context(), venueID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, items)
}

// Response DTO since domain have bookingID string but consumer see encoded string
type ActiveGameResponseItem struct {
	BookingID     string    `json:"booking_id"`
	VenueID       int64     `json:"venue_id"`
	UserID        int64     `json:"user_id"`
	CustomerName  *string   `json:"customer_name,omitempty"`
	CustomerPhone *string   `json:"customer_phone,omitempty"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	TotalPrice    int       `json:"total_price"`
	Status        string    `json:"status"`
}

// listActiveGamesHandler godoc
//
//	@Summary		List active games for venue
//	@Description	Returns games/bookings currently happening at the venue.
//	@Tags			venue games
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int					true	"Venue ID"
//	@Success		200		{object}	ActiveGamesResponse	"Active games"
//	@Failure		400		{object}	ErrorResponse		"Bad request"
//	@Failure		404		{object}	ErrorResponse		"Venue not found"
//	@Failure		500		{object}	ErrorResponse		"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/games/active [get]
func (app *application) listActiveGamesHandler(w http.ResponseWriter, r *http.Request) {
	venueID, err := readIDParam(r, "venueID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	games, err := app.store.Inventory.ListActiveGames(r.Context(), venueID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	resp := make([]ActiveGameResponseItem, 0, len(games))

	for _, game := range games {
		resp = append(resp, ActiveGameResponseItem{
			BookingID:     app.EncodeBookingID(game.BookingID),
			VenueID:       game.VenueID,
			UserID:        game.UserID,
			CustomerName:  game.CustomerName,
			CustomerPhone: game.CustomerPhone,
			StartTime:     game.StartTime,
			EndTime:       game.EndTime,
			TotalPrice:    game.TotalPrice,
			Status:        game.Status,
		})
	}

	app.jsonResponse(w, http.StatusOK, resp)
}

// Response DTO to encode booking_id string
type GameDetailResponseItem struct {
	Game           ActiveGameResponseItem           `json:"game"`
	Inventory      []inventory.InventoryItem        `json:"inventory_items"`
	AddedItems     []inventory.BookingInventoryItem `json:"added_items"`
	BillingSummary inventory.BillingSummary         `json:"billing_summary"`
}

// getGameDetailHandler godoc
//
//	@Summary		Get game detail
//	@Description	Returns game detail, inventory items, added items, and billing summary.
//	@Tags			venue games
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int					true	"Venue ID"
//	@Param			bookingID	path		string				true	"Encoded Booking ID"
//	@Success		200			{object}	GameDetailResponse	"Game detail"
//	@Failure		400			{object}	ErrorResponse		"Bad request"
//	@Failure		404			{object}	ErrorResponse		"Game not found"
//	@Failure		500			{object}	ErrorResponse		"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/games/{bookingID} [get]
func (app *application) getGameDetailHandler(w http.ResponseWriter, r *http.Request) {
	venueID, err := readIDParam(r, "venueID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	bookingIDParam := chi.URLParam(r, "bookingID")

	bookingID, err := app.parseBookingParam(bookingIDParam)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	detail, err := app.store.Inventory.GetGameDetail(r.Context(), venueID, bookingID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	resp := GameDetailResponseItem{
		Game: ActiveGameResponseItem{
			BookingID:     app.EncodeBookingID(detail.Game.BookingID),
			VenueID:       detail.Game.VenueID,
			UserID:        detail.Game.UserID,
			CustomerName:  detail.Game.CustomerName,
			CustomerPhone: detail.Game.CustomerPhone,
			StartTime:     detail.Game.StartTime,
			EndTime:       detail.Game.EndTime,
			TotalPrice:    detail.Game.TotalPrice,
			Status:        detail.Game.Status,
		},
		Inventory:      detail.Inventory,
		AddedItems:     detail.AddedItems,
		BillingSummary: detail.BillingSummary,
	}

	app.jsonResponse(w, http.StatusOK, resp)
}

type BookingInventoryItemResponseItem struct {
	ID                int64     `json:"id"`
	BookingID         string    `json:"booking_id"`
	VenueID           int64     `json:"venue_id"`
	InventoryItemID   int64     `json:"inventory_item_id"`
	ItemNameSnapshot  string    `json:"item_name"`
	UnitPriceSnapshot int       `json:"unit_price"`
	Quantity          int       `json:"quantity"`
	LineTotal         int       `json:"line_total"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type AddItemToGameData struct {
	AddedItem      BookingInventoryItemResponseItem `json:"added_item"`
	BillingSummary inventory.BillingSummary         `json:"billing_summary"`
}

// addItemToGameHandler godoc
//
//	@Summary		Add inventory item to active game
//	@Description	Adds an inventory item to a currently active game. If item already exists, quantity is increased.
//	@Tags			venue games
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int									true	"Venue ID"
//	@Param			bookingID	path		string								true	"Encoded Booking ID"
//	@Param			payload		body		inventory.AddItemToBookingPayload	true	"Inventory item and quantity"
//	@Success		200			{object}	AddItemToGameResponse				"Added item and updated billing summary"
//	@Failure		400			{object}	ErrorResponse						"Bad request"
//	@Failure		404			{object}	ErrorResponse						"Inventory item or game not found"
//	@Failure		500			{object}	ErrorResponse						"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/games/{bookingID}/items [post]
func (app *application) addItemToGameHandler(w http.ResponseWriter, r *http.Request) {
	venueID, err := readIDParam(r, "venueID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	bookingIDParam := chi.URLParam(r, "bookingID")

	bookingID, err := app.parseBookingParam(bookingIDParam)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	var payload inventory.AddItemToBookingPayload

	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if payload.Quantity <= 0 {
		payload.Quantity = 1
	}

	addedItem, err := app.store.Inventory.AddItemToBooking(
		r.Context(),
		venueID,
		bookingID,
		payload.InventoryItemID,
		payload.Quantity,
	)
	if err != nil {
		if errors.Is(err, inventory.ErrBookingNotActive) ||
			errors.Is(err, inventory.ErrInventoryItemNotFound) {
			app.badRequestResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	summary, err := app.store.Inventory.GetBillingSummary(r.Context(), venueID, bookingID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	resp := AddItemToGameData{
		AddedItem: BookingInventoryItemResponseItem{
			ID:                addedItem.ID,
			BookingID:         app.EncodeBookingID(addedItem.BookingID),
			VenueID:           addedItem.VenueID,
			InventoryItemID:   addedItem.InventoryItemID,
			ItemNameSnapshot:  addedItem.ItemNameSnapshot,
			UnitPriceSnapshot: addedItem.UnitPriceSnapshot,
			Quantity:          addedItem.Quantity,
			LineTotal:         addedItem.LineTotal,
			CreatedAt:         addedItem.CreatedAt,
			UpdatedAt:         addedItem.UpdatedAt,
		},
		BillingSummary: *summary,
	}

	app.jsonResponse(w, http.StatusOK, resp)
}

// updateInventoryItemHandler godoc
//
//	@Summary		Update venue inventory item
//	@Description	Updates an existing inventory item for a venue. Supports updating text fields and replacing image.
//	@Tags			venue inventory
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			venueID			path		int						true	"Venue ID"
//	@Param			itemID			path		int						true	"Inventory Item ID"
//	@Param			name			formData	string					false	"Inventory item name"
//	@Param			description		formData	string					false	"Inventory item description"
//	@Param			unit_price		formData	int						false	"Unit price"
//	@Param			stock_quantity	formData	int						false	"Stock quantity"
//	@Param			track_stock		formData	bool					false	"Track stock"
//	@Param			is_active		formData	bool					false	"Is active"
//	@Param			image			formData	file					false	"Inventory item image"
//	@Success		200				{object}	InventoryItemResponse	"Inventory item updated"
//	@Failure		400				{object}	ErrorResponse			"Bad request"
//	@Failure		404				{object}	ErrorResponse			"Inventory item not found"
//	@Failure		500				{object}	ErrorResponse			"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/inventory/{itemID} [patch]
func (app *application) updateInventoryItemHandler(w http.ResponseWriter, r *http.Request) {
	venueID, err := readIDParam(r, "venueID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	itemID, err := readIDParam(r, "itemID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Get old item first so we can delete old Cloudinary image after successful DB update.
	oldItem, err := app.store.Inventory.GetInventoryItemByID(r.Context(), venueID, itemID)
	if err != nil {
		if errors.Is(err, inventory.ErrInventoryItemNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	var payload inventory.UpdateInventoryItemPayload

	if nameValue := strings.TrimSpace(r.FormValue("name")); nameValue != "" {
		if len(nameValue) > 100 {
			app.badRequestResponse(w, r, errors.New("name cannot be more than 100 characters"))
			return
		}

		payload.Name = &nameValue
	}

	if descriptionValue := strings.TrimSpace(r.FormValue("description")); descriptionValue != "" {
		if len(descriptionValue) > 255 {
			app.badRequestResponse(w, r, errors.New("description cannot be more than 255 characters"))
			return
		}

		payload.Description = &descriptionValue
	}

	if unitPriceValue := strings.TrimSpace(r.FormValue("unit_price")); unitPriceValue != "" {
		unitPrice, err := strconv.Atoi(unitPriceValue)
		if err != nil {
			app.badRequestResponse(w, r, errors.New("invalid unit_price"))
			return
		}

		if unitPrice < 0 {
			app.badRequestResponse(w, r, errors.New("unit_price cannot be negative"))
			return
		}

		payload.UnitPrice = &unitPrice
	}

	if stockQuantityValue := strings.TrimSpace(r.FormValue("stock_quantity")); stockQuantityValue != "" {
		stockQuantity, err := strconv.Atoi(stockQuantityValue)
		if err != nil {
			app.badRequestResponse(w, r, errors.New("invalid stock_quantity"))
			return
		}

		if stockQuantity < 0 {
			app.badRequestResponse(w, r, errors.New("stock_quantity cannot be negative"))
			return
		}

		payload.StockQuantity = &stockQuantity
	}

	if trackStockValue := strings.TrimSpace(r.FormValue("track_stock")); trackStockValue != "" {
		trackStock, err := strconv.ParseBool(trackStockValue)
		if err != nil {
			app.badRequestResponse(w, r, errors.New("invalid track_stock"))
			return
		}

		payload.TrackStock = &trackStock
	}

	if isActiveValue := strings.TrimSpace(r.FormValue("is_active")); isActiveValue != "" {
		isActive, err := strconv.ParseBool(isActiveValue)
		if err != nil {
			app.badRequestResponse(w, r, errors.New("invalid is_active"))
			return
		}

		payload.IsActive = &isActive
	}

	var newImageURL *string

	file, fileHeader, err := r.FormFile("image")
	if err == nil {
		defer file.Close()

		// Future reference:
		// Always validate image type before uploading to Cloudinary.
		if !app.isValidAdImageType(fileHeader.Header.Get("Content-Type")) {
			app.badRequestResponse(w, r, errors.New("invalid image type"))
			return
		}

		itemNameForPublicID := oldItem.Name
		if payload.Name != nil {
			itemNameForPublicID = *payload.Name
		}

		uploadedURL, err := app.uploadInventoryImageToCloudinary(file, venueID, itemNameForPublicID)
		if err != nil {
			app.internalServerError(w, r, err)
			return
		}

		newImageURL = &uploadedURL
		payload.ImageURL = newImageURL
	} else if !errors.Is(err, http.ErrMissingFile) {
		app.badRequestResponse(w, r, err)
		return
	}

	err = app.store.Inventory.UpdateInventoryItem(r.Context(), venueID, itemID, payload)
	if err != nil {
		// Future reference:
		// If DB update fails after new Cloudinary upload, delete the newly uploaded image
		// to avoid leaving unused images in Cloudinary.
		if newImageURL != nil {
			go func(url string) {
				if err := app.deletePhotoFromCloudinary(url); err != nil {
					app.logger.Error("failed to delete newly uploaded inventory image from Cloudinary", "url", url, "err", err)
				}
			}(*newImageURL)
		}

		if errors.Is(err, inventory.ErrInventoryItemNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	// Future reference:
	// Delete old Cloudinary image only after DB update succeeds.
	// This prevents broken image_url in DB if database update fails.
	if newImageURL != nil && oldItem.ImageURL != nil && *oldItem.ImageURL != "" {
		go func(url string) {
			if err := app.deletePhotoFromCloudinary(url); err != nil {
				app.logger.Error("failed to delete old inventory image from Cloudinary", "url", url, "err", err)
			}
		}(*oldItem.ImageURL)
	}

	updatedItem, err := app.store.Inventory.GetInventoryItemByID(r.Context(), venueID, itemID)
	if err != nil {
		if errors.Is(err, inventory.ErrInventoryItemNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, updatedItem)
}

// deleteInventoryItemHandler godoc
//
//	@Summary		Delete venue inventory item
//	@Description	Soft deletes an inventory item by setting is_active to false.
//	@Tags			venue inventory
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int				true	"Venue ID"
//	@Param			itemID	path		int				true	"Inventory Item ID"
//	@Success		200		{object}	MessageResponse	"Inventory item deleted"
//	@Failure		400		{object}	ErrorResponse	"Bad request"
//	@Failure		404		{object}	ErrorResponse	"Inventory item not found"
//	@Failure		500		{object}	ErrorResponse	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/inventory/{itemID} [delete]
func (app *application) deleteInventoryItemHandler(w http.ResponseWriter, r *http.Request) {
	venueID, err := readIDParam(r, "venueID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	itemID, err := readIDParam(r, "itemID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Future reference:
	// This should be a soft delete in repository:
	// UPDATE venue_inventory_items SET is_active = FALSE
	// Do not hard delete because old booking bills may still reference this item.
	err = app.store.Inventory.DeleteInventoryItem(r.Context(), venueID, itemID)
	if err != nil {
		if errors.Is(err, inventory.ErrInventoryItemNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	resp := map[string]string{
		"message": "inventory item deleted successfully",
	}

	app.jsonResponse(w, http.StatusOK, resp)
}
