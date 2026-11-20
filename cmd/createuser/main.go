// Command createuser creates a DM or player account.
//
//	DATABASE_URL=file:craftingo.db go run ./cmd/createuser -username dm -password secret -role dm
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"github.com/Nico-DM/craftingo/internal/auth"
	"github.com/Nico-DM/craftingo/internal/catalog"
	"github.com/Nico-DM/craftingo/internal/database"
)

func main() {
	username := flag.String("username", "", "account name")
	password := flag.String("password", "", "password (min 6 chars)")
	role := flag.String("role", "player", "dm or player")
	flag.Parse()

	if *username == "" || len(*password) < 6 || (*role != "dm" && *role != "player") {
		fmt.Println("usage: createuser -username NAME -password PASS -role dm|player")
		os.Exit(2)
	}

	_ = godotenv.Load(".env")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}
	db, err := catalog.OpenDB(dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	hash, err := auth.HashPassword(*password)
	if err != nil {
		log.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	u, err := database.For(db).CreateUser(context.Background(), database.CreateUserParams{
		ID: uuid.NewString(), Username: *username, PasswordHash: hash, Role: *role,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("created %s (%s)\n", u.Username, u.Role)
}
