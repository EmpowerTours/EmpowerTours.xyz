package models

import "time"

type Experience struct {
	ID            string     `json:"id" db:"id"`
	Slug          *string    `json:"slug" db:"slug"`
	Title         string     `json:"title" db:"title"`
	Category      string     `json:"category" db:"category"`
	Description   *string    `json:"description" db:"description"`
	LocationName  *string    `json:"locationName" db:"location_name"`
	Latitude      *float64   `json:"latitude" db:"latitude"`
	Longitude     *float64   `json:"longitude" db:"longitude"`
	PriceMon      *float64   `json:"priceMon" db:"price_mon"`
	PriceUSD      *float64   `json:"priceUsd" db:"price_usd"`
	MaxGuests     *int       `json:"maxGuests" db:"max_guests"`
	MinGuests     *int       `json:"minGuests" db:"min_guests"`
	ImageURL      *string    `json:"imageUrl" db:"image_url"`
	IsActive      bool       `json:"isActive" db:"is_active"`
	CreatorID     *string    `json:"creatorId" db:"creator_id"`
	Status        string     `json:"status" db:"status"`
	DurationMin   *int       `json:"durationMin" db:"duration_min"`
	MeetingPoint  *string    `json:"meetingPoint" db:"meeting_point"`
	WhatToBring   *string    `json:"whatToBring" db:"what_to_bring"`
	Languages     *string    `json:"languages" db:"languages"`
	CoverPhotoURL *string    `json:"coverPhotoUrl" db:"cover_photo_url"`
	GalleryURLs   *string    `json:"galleryUrls" db:"gallery_urls"`
	CreatedAt     time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt     *time.Time `json:"updatedAt" db:"updated_at"`
}
