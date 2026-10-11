// Command erase-user erases one account and the data only that account owns.
//
// Dry run (default):
//
//	DATABASE_URL=... erase-user -email person@example.com
//
// Execute after reading the inventory:
//
//	DATABASE_URL=... erase-user -email person@example.com -execute \
//	  -confirm person@example.com -confirm-database-host <host from the dry run>
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/gdpr"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/pgdb"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "erase-user:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		email, userID, confirm, confirmHost string
		execute                             bool
	)
	flag.StringVar(&email, "email", "", "email of the user to erase")
	flag.StringVar(&userID, "user-id", "", "id of the user to erase")
	flag.BoolVar(&execute, "execute", false, "erase for real; without it only the inventory is printed")
	flag.StringVar(&confirm, "confirm", "", "the exact email of the user, required with -execute")
	flag.StringVar(&confirmHost, "confirm-database-host", "", "the database host printed by the dry run, required with -execute")
	flag.Parse()

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	host := parsed.Hostname()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := pgdb.Open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	eraser := gdpr.Eraser{DB: pool}

	subject, err := eraser.Find(ctx, strings.TrimSpace(email), strings.TrimSpace(userID))
	if err != nil {
		return err
	}
	plan, err := eraser.Plan(ctx, subject)
	if err != nil {
		return err
	}

	fmt.Printf("database host: %s\nuser: %s <%s> (%s)\n", host, subject.Name, subject.Email, subject.ID)
	for _, o := range plan.Organizations {
		fmt.Printf("organization to erase: %s (%s)\n", o.Name, o.ID)
	}
	fmt.Printf("projects to erase: %d\n", len(plan.ProjectIDs))
	keys := make([]string, 0, len(plan.Rows))
	for k := range plan.Rows {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		fmt.Printf("  %-55s %d rows\n", k, plan.Rows[k])
	}

	if !execute {
		fmt.Println("\ndry run only. Re-run with -execute -confirm <email> -confirm-database-host", host)
		return nil
	}
	if !strings.EqualFold(confirm, subject.Email) {
		return errors.New("-confirm must be the exact email of the user")
	}
	if confirmHost != host {
		return errors.New("-confirm-database-host must match the database host printed above")
	}
	if err := eraser.Execute(ctx, plan); err != nil {
		return err
	}
	fmt.Println("erased.")
	return nil
}
