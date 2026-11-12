package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/api"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/changelog"
	"github.com/WiFiGUY512/aeolus/internal/hierarchy"
	"github.com/WiFiGUY512/aeolus/internal/schema"
	"github.com/WiFiGUY512/aeolus/internal/secret"
)

// runInit creates the Org. It runs once, on the manager host, by the person
// who becomes the first admin, its owner (0176): every change it makes is
// logged under that admin's name. With -password-stdin it sets the admin's
// password, read from the first line of standard input, as the installer
// hands it on, and prints no token unless -token asks for one; without, it
// prints the admin's token, only to their terminal.
//
// With -mcp-account, it also creates the MCP adapter's account (0028) and
// writes that account's token to a file, never to the terminal, so no token
// passes through a chat.
func runInit(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	db := fs.String("db", "/var/lib/aeolus/aeolus.db", "change log database")
	keyPath := fs.String("key", "/etc/aeolus/secret.key", "secret key file (created if missing)")
	orgID := fs.String("org", "", "Org ID, e.g. example")
	orgName := fs.String("org-name", "", "Org display name")
	admin := fs.String("admin", "", "first admin's account ID; you, running this")
	mcpAccount := fs.String("mcp-account", "", "account for the MCP adapter, e.g. claude (optional)")
	mcpRole := fs.String("mcp-role", "operator", "role for the MCP adapter at both roots: viewer, operator, manager, installer or admin")
	mcpToken := fs.String("mcp-token", "/etc/aeolus/mcp.token", "file to write the MCP adapter's token to")
	passwordStdin := fs.Bool("password-stdin", false, "set the first admin's password, read from the first line of standard input; no token is printed unless -token")
	printToken := fs.Bool("token", false, "print an API token for the first admin as well, for scripts")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *orgID == "" || *admin == "" {
		return errors.New("init needs -org and -admin")
	}
	// The password is read and checked before anything is made, so one
	// that won't do leaves no Org behind.
	password := ""
	if *passwordStdin {
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("reading the password: %w", err)
		}
		password = strings.TrimRight(line, "\r\n")
		if err := access.CheckPasswordRules(access.AccountID(*admin), password); err != nil {
			return err
		}
	}
	if *orgName == "" {
		*orgName = *orgID
	}
	role := access.None
	if *mcpAccount != "" {
		var err error
		if role, err = access.ParseRole(*mcpRole); err != nil {
			return err
		}
		if _, err := os.Stat(*mcpToken); err == nil {
			return fmt.Errorf("%s already exists; refusing to overwrite a token file", *mcpToken)
		}
	}

	sch, err := schema.V1()
	if err != nil {
		return err
	}
	if _, err := secret.LoadOrCreate(*keyPath); err != nil {
		return fmt.Errorf("secret key: %w", err)
	}
	log, err := changelog.Open(*db, changelog.Options{Check: api.Check(sch)})
	if err != nil {
		return err
	}
	defer log.Close()
	if log.Snapshot() != nil {
		return errors.New("this change log already holds an Org")
	}

	as := func(op change.Op) error {
		_, err := log.Commit(*admin, "aeolus init", op)
		return err
	}
	root := hierarchy.NodeID(*orgID)
	if err := as(change.Op{Kind: change.CreateOrg, Node: root, Name: *orgName, Account: access.AccountID(*admin)}); err != nil {
		return err
	}
	if password != "" {
		hash, err := access.HashPassword(password)
		if err != nil {
			return err
		}
		if err := as(change.Op{Kind: change.SetPassword, Account: access.AccountID(*admin), PasswordHash: hash}); err != nil {
			return err
		}
	}
	adminToken := ""
	if password == "" || *printToken {
		if adminToken, err = issue(as, access.AccountID(*admin)); err != nil {
			return err
		}
	}

	if *mcpAccount != "" {
		acct := access.AccountID(*mcpAccount)
		if err := as(change.Op{Kind: change.AddAccount, Account: acct, Name: "MCP adapter (" + *mcpAccount + ")"}); err != nil {
			return err
		}
		for _, tree := range []change.TreeName{change.Locations, change.Services} {
			if err := as(change.Op{Kind: change.GrantRole, Account: acct, Tree: tree, Node: root, Role: role.String()}); err != nil {
				return err
			}
		}
		token, err := issue(as, acct)
		if err != nil {
			return err
		}
		if err := os.WriteFile(*mcpToken, []byte(token+"\n"), 0o600); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "MCP adapter account %q is %s at both roots; its token is in %s (not shown).\n", *mcpAccount, role, *mcpToken)
	}

	fmt.Fprintf(stdout, "Org %q created; %s is its super admin, and admin of both trees.\n", *orgID, *admin)
	if password != "" {
		fmt.Fprintf(stdout, "Sign in as %s with the password you chose.\n", *admin)
	}
	if adminToken != "" {
		fmt.Fprintf(stdout, "\nYour API token (shown once, keep it safe):\n\n  %s\n\n", adminToken)
	}
	return nil
}

// issue makes a token for an account and returns its plain text.
func issue(as func(change.Op) error, account access.AccountID) (string, error) {
	plain, id, hash, err := access.NewToken()
	if err != nil {
		return "", err
	}
	return plain, as(change.Op{Kind: change.IssueToken, Account: account, TokenID: id, TokenHash: hash})
}
