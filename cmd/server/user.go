package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"

	"go-simpler.org/env"
	"golang.org/x/term"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
	"github.com/iuya/battle-company-manager-ce-v2/internal/postgres"
)

const userUsage = `usage: server user create --email <email> --name <display name>
       server user grant  --email <email> [--role user|admin|superuser] [--create-campaigns]

Creates an account (there is no sign-up page). The password is read from the terminal, or from the first line of
stdin when it is piped. "grant" sets the global role and whether a plain user may create campaigns.`

// runUser handles "server user create".
func runUser(args []string) error {
	if len(args) > 0 && args[0] == "grant" {
		return runGrant(args[1:])
	}
	if len(args) == 0 || args[0] != "create" {
		return errors.New(userUsage)
	}
	fs := flag.NewFlagSet("user create", flag.ContinueOnError)
	email := fs.String("email", "", "email address used to sign in")
	name := fs.String("name", "", "display name")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *email == "" || *name == "" {
		return errors.New(userUsage)
	}
	var cfg config
	if err := env.Load(&cfg, nil); err != nil {
		return err
	}
	password, err := readPassword()
	if err != nil {
		return err
	}

	ctx := context.Background()
	store, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	user, err := core.NewAuth(store).CreateUser(ctx, *email, *name, password)
	switch {
	case errors.Is(err, core.ErrEmailTaken):
		return fmt.Errorf("%s is already registered", *email)
	case err != nil:
		return err
	}
	fmt.Printf("Created %s <%s>\n", user.DisplayName, user.Email)
	return nil
}

func readPassword() (string, error) {
	fd := int(syscall.Stdin)
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", errors.New("no password on stdin")
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fmt.Fprint(os.Stderr, "Password: ")
	first, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "Repeat password: ")
	second, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if string(first) != string(second) {
		return "", errors.New("the passwords do not match")
	}
	return string(first), nil
}

// runGrant handles "server user grant".
func runGrant(args []string) error {
	fs := flag.NewFlagSet("user grant", flag.ContinueOnError)
	email := fs.String("email", "", "account to change")
	role := fs.String("role", string(core.RoleUser), "global role: user, admin or superuser")
	create := fs.Bool("create-campaigns", false, "let a plain user create campaigns")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *email == "" {
		return errors.New(userUsage)
	}
	var cfg config
	if err := env.Load(&cfg, nil); err != nil {
		return err
	}
	ctx := context.Background()
	store, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	switch err := core.NewAuth(store).SetPermissions(ctx, *email, core.Role(*role), *create); {
	case errors.Is(err, core.ErrNotFound):
		return fmt.Errorf("%s is not registered", *email)
	case err != nil:
		return err
	}
	fmt.Printf("%s is now %s (create campaigns: %t)\n", *email, *role, *create)
	return nil
}
