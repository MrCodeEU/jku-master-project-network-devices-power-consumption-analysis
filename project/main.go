package main

import (
	"flag"
	"log"
	"os"

	"project/internal/database"
	"project/internal/fritzbox"
	"project/internal/loadgen"
	"project/internal/runner"
	"project/internal/server"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using defaults or flags")
	}

	addr := flag.String("addr", ":8080", "Address to listen on")
	mock := flag.Bool("mock", false, "Use mock power meter")
	flag.Parse()

	var meter fritzbox.PowerMeter
	if *mock {
		log.Println("Using Mock Power Meter")
		meter = fritzbox.NewMockPowerMeter()
	} else {
		log.Println("Using Real Power Meter")

		url := os.Getenv("FRITZ_URL")
		user := os.Getenv("FRITZ_USER")
		pass := os.Getenv("FRITZ_PASSWORD")
		ain := os.Getenv("FRITZ_AIN")

		if url == "" {
			url = "http://fritz.box:49000"
		}

		meter = fritzbox.NewRealPowerMeter(url, user, pass, ain)
	}

	lg := loadgen.NewNetworkLoadGenerator()
	r := runner.NewRunner(meter, lg)

	// Initialize database
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "tests.db"
	}
	db, err := database.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()
	log.Printf("Database initialized: %s", dbPath)

	srv := server.NewServer(r, db)

	log.Printf("Starting server on %s", *addr)
	if err := srv.Start(*addr); err != nil {
		log.Fatal(err)
	}
}
