package helpers

import (
	"khel/internal/domain/adminview"
	"khel/internal/domain/bookings"
	"khel/internal/domain/games"
	"khel/internal/domain/orders"
	"khel/internal/domain/users"
)

func ToAdminUserDTO(u users.User) adminview.UserDTO {
	var pic *string
	if u.ProfilePictureURL.Valid {
		pic = &u.ProfilePictureURL.String
	}

	var skill *string
	if u.SkillLevel.Valid {
		skill = &u.SkillLevel.String
	}

	var nog *int
	if u.NoOfGames.Valid {
		v := int(u.NoOfGames.Int16)
		nog = &v
	}

	return adminview.UserDTO{
		ID:                u.ID,
		FirstName:         u.FirstName,
		LastName:          u.LastName,
		Email:             u.Email,
		Phone:             u.Phone,
		ProfilePictureURL: pic,
		SkillLevel:        skill,
		NoOfGames:         nog,
		IsActive:          u.IsActive,
		CreatedAt:         u.CreatedAt,
		UpdatedAt:         u.UpdatedAt,
	}
}

func ToAdminUserStatsDTO(s users.AdminUserStatsRow) adminview.UserStats {
	return adminview.UserStats{
		OrdersCount:     s.OrdersCount,
		BookingsCount:   s.BookingsCount,
		GamesCount:      s.GamesCount,
		TotalSpentCents: s.TotalSpentCents,
		LastOrderAt:     s.LastOrderAt,
		LastBookingAt:   s.LastBookingAt,
		LastGameAt:      s.LastGameAt,
	}
}

func ToOrderDTO(o orders.Order) adminview.OrderDTO {
	return adminview.OrderDTO{
		ID:            o.ID,
		OrderNumber:   o.OrderNumber,
		Status:        o.Status,
		PaymentStatus: o.PaymentStatus,
		TotalCents:    o.TotalCents,
		CreatedAt:     o.CreatedAt,
	}
}

func ToBookingDTO(b bookings.UserBooking) adminview.BookingDTO {
	return adminview.BookingDTO{
		BookingID:    b.BookingID,
		VenueID:      b.VenueID,
		VenueName:    b.VenueName,
		VenueAddress: b.VenueAddress,
		StartTime:    b.StartTime,
		EndTime:      b.EndTime,
		TotalPrice:   b.TotalPrice,
		Status:       b.Status,
		CreatedAt:    b.CreatedAt,
	}
}

func ToGameDTO(g games.GameSummary) adminview.GameDTO {
	return adminview.GameDTO{
		ID:        g.GameID,
		SportType: g.SportType,
		VenueID:   g.VenueID,
		StartTime: g.StartTime,
		EndTime:   g.EndTime,
		Status:    g.Status,
	}
}
