package services

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type seedExperience struct {
	Title        string
	Slug         string
	Category     string
	LocationName string
	PriceMon     float64
	PriceUSD     float64
	Description  string
}

// SeedExperiences inserts local demo experiences. It must only be called in
// explicitly enabled development environments.
func SeedExperiences(db *sqlx.DB) error {
	var count int
	err := db.Get(&count, "SELECT COUNT(*) FROM experiences")
	if err != nil {
		return fmt.Errorf("failed to count experiences: %w", err)
	}
	if count > 0 {
		return nil
	}

	seeds := []seedExperience{
		{
			Title:        "Ranch Day at Tierra Colorada",
			Slug:         "ranch-day-at-tierra-colorada",
			Category:     "ranch",
			LocationName: "Tierra Colorada, Guerrero",
			PriceMon:     500,
			PriceUSD:     25,
			Description:  "Private huerta across a small river. Ride a Yamaha Blaster 200, hang with cows, goats, chickens, ducks, exotic birds, and dogs. A total escape from the ordinary.",
		},
		{
			Title:        "Rock Climbing in Villa Guerrero",
			Slug:         "rock-climbing-in-villa-guerrero",
			Category:     "climbing",
			LocationName: "Villa Guerrero, Guerrero",
			PriceMon:     350,
			PriceUSD:     18,
			Description:  "Natural rock formations for all levels. Explore stunning landscape with options to rent or purchase land.",
		},
		{
			Title:        "Coastal Dining in Acapulco",
			Slug:         "coastal-dining-in-acapulco",
			Category:     "dining",
			LocationName: "Acapulco, Guerrero",
			PriceMon:     750,
			PriceUSD:     38,
			Description:  "Beachfront restaurants, fresh Pacific seafood, nightlife. Acapulco like the locals know it.",
		},
		{
			Title:        "Private Transport",
			Slug:         "private-transport",
			Category:     "transport",
			LocationName: "Guerrero",
			PriceMon:     150,
			PriceUSD:     8,
			Description:  "Safe, comfortable rides to every corner of the state. Airport transfers, day trips, multi-stop journeys.",
		},
	}

	now := time.Now()
	for _, s := range seeds {
		id := uuid.New().String()
		_, err := db.Exec(`INSERT INTO experiences (id, slug, title, category, description, location_name, price_mon, price_usd, is_active, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?)`,
			id, s.Slug, s.Title, s.Category, s.Description, s.LocationName, s.PriceMon, s.PriceUSD, now)
		if err != nil {
			return fmt.Errorf("failed to seed experience %q: %w", s.Title, err)
		}
		fmt.Printf("Seeded experience: %s\n", s.Title)
	}

	return nil
}
