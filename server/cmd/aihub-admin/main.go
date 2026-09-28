// aihub-admin initializes the sole first administrator. Supply the password on stdin.
package main

import (
	"aihub.dev/server/internal/api"
	"aihub.dev/server/internal/db"
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	email := flag.String("email", "", "administrator email")
	flag.Parse()
	dsn := os.Getenv("AIHUB_DATABASE_URL")
	if dsn == "" || *email == "" {
		fmt.Fprintln(os.Stderr, "AIHUB_DATABASE_URL and --email are required")
		os.Exit(2)
	}
	info, e := os.Stdin.Stat()
	if e != nil || info.Mode()&os.ModeCharDevice != 0 {
		fmt.Fprintln(os.Stderr, "pipe a hidden password to stdin; interactive echo is disabled")
		os.Exit(2)
	}
	reader := bufio.NewReader(os.Stdin)
	password, e := reader.ReadString('\n')
	if e != nil {
		fmt.Fprintln(os.Stderr, "password input failed")
		os.Exit(2)
	}
	password = strings.TrimRight(password, "\r\n")
	database, e := db.Open(dsn)
	if e != nil {
		fmt.Fprintln(os.Stderr, "database unavailable")
		os.Exit(1)
	}
	defer database.Close()
	ctx := context.Background()
	if e = db.Migrate(ctx, database); e != nil {
		fmt.Fprintln(os.Stderr, "migration failed")
		os.Exit(1)
	}
	if e = api.BootstrapAdmin(ctx, database, *email, password); e != nil {
		fmt.Fprintln(os.Stderr, e.Error())
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "Administrator initialized")
}
