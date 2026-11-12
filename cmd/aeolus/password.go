package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/WiFiGUY512/aeolus/internal/access"
	"github.com/WiFiGUY512/aeolus/internal/api"
	"github.com/WiFiGUY512/aeolus/internal/change"
	"github.com/WiFiGUY512/aeolus/internal/changelog"
	"github.com/WiFiGUY512/aeolus/internal/schema"
)

// runPassword sets a temporary password for an account, on the manager
// host (0176): the last way back in, when no owner can reset it. Access to
// the host is the trust, as for aeolus token (0043). The change is logged
// under the account itself, which may always set its own password, and the
// password is temporary: it is changed at the next sign-in.
//
// The service must be stopped while it runs: the change log admits one
// process at a time.
func runPassword(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("password", flag.ContinueOnError)
	fs.SetOutput(stderr)
	db := fs.String("db", "/var/lib/aeolus/aeolus.db", "change log database")
	account := fs.String("account", "", "the account to set a temporary password for")
	reason := fs.String("reason", "temporary password set on the manager host (aeolus password)", "why, for the change log")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *account == "" {
		return errors.New("password needs -account")
	}
	if err := checkOwner(*db); err != nil {
		return err
	}
	sch, err := schema.V1()
	if err != nil {
		return err
	}
	log, err := changelog.Open(*db, changelog.Options{Check: api.Check(sch)})
	if err != nil {
		return err
	}
	defer log.Close()
	state := log.Snapshot()
	if state == nil {
		return errors.New("the change log holds no Org yet; run aeolus init")
	}
	acct := access.AccountID(*account)
	acc, ok := state.Access.Account(acct)
	if !ok {
		return fmt.Errorf("no account %q", *account)
	}
	if !acc.Active() {
		return fmt.Errorf("%s is disabled or removed: an owner enables it first", *account)
	}
	plain, err := access.TemporaryPassword()
	if err != nil {
		return err
	}
	hash, err := access.HashPassword(plain)
	if err != nil {
		return err
	}
	if _, err := log.Commit(*account, *reason, change.Op{Kind: change.SetPassword, Account: acct, PasswordHash: hash, MustChange: true}); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Temporary password for %s (shown once; it is changed at the first sign-in):\n\n  %s\n\n", *account, plain)
	fmt.Fprintln(stdout, "Start the service again: systemctl start aeolus")
	return nil
}
