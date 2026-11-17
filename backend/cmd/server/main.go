package main

import (
	"log"
	"os"

	"certificate-console/backend/internal/app"
)

func main() {
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = "./certificates.db"
	}
	db, err := app.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := app.Migrate(db); err != nil {
		log.Fatal(err)
	}
	if err := app.Seed(db); err != nil {
		log.Fatal(err)
	}
	if len(os.Args) > 1 && os.Args[1] == "seed-large" {
		if err := app.SeedLargeCLI(db, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Fatal(app.Router(db).Run(addr))
}
