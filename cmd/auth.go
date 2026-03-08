package cmd

import (
	"fmt"
	"os"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/internal/auth"
	"golang.org/x/term"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage OnlyAgents authentication",
}

// onlyagents auth reset
// Generates a new random password and prints it.
// Use when you've forgotten your password.
var authResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset password — generates a new random password",
	Long: `Generates a new random password, saves the bcrypt hash to auth.yaml,
and invalidates all active sessions. Use when you've forgotten your password.

The new password is printed once. Save it somewhere safe.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := dataDir()
		newPass, err := auth.ResetPassword(dir)
		if err != nil {
			return fmt.Errorf("resetting password: %w", err)
		}
		fmt.Println()
		fmt.Println("Password reset successfully.")
		fmt.Printf("New password: %s\n", newPass)
		fmt.Println()
		fmt.Println("All existing sessions have been invalidated.")
		fmt.Println("Log in again at your OnlyAgents server.")
		fmt.Println()
		return nil
	},
}

// onlyagents auth set-password
// Interactive password change from the CLI (for when you know your current password).
var authSetPasswordCmd = &cobra.Command{
	Use:   "set-password",
	Short: "Change password interactively",
	Long:  `Prompts for your current password and a new password, then updates auth.yaml.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := dataDir()

		fmt.Print("Current password: ")
		currentRaw, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("reading password: %w", err)
		}

		fmt.Print("New password (min 8 chars): ")
		newRaw, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("reading new password: %w", err)
		}

		fmt.Print("Confirm new password: ")
		confirmRaw, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("reading confirmation: %w", err)
		}

		if string(newRaw) != string(confirmRaw) {
			fmt.Fprintln(os.Stderr, "Error: passwords do not match")
			os.Exit(1)
		}

		// Use Auth.ChangePassword so sessions are invalidated atomically
		limiter := auth.NewIPRateLimiter(0, 0) // no rate limit for CLI
		a := auth.New(dir, limiter)

		err = a.ChangePassword(auth.ChangePasswordRequest{
			CurrentPassword: string(currentRaw),
			NewPassword:     string(newRaw),
		})
		if err != nil {
			switch err {
			case auth.ErrBadCredentials:
				fmt.Fprintln(os.Stderr, "Error: current password is incorrect")
				os.Exit(1)
			case auth.ErrPasswordTooShort:
				fmt.Fprintln(os.Stderr, "Error: new password must be at least 8 characters")
				os.Exit(1)
			default:
				return err
			}
		}

		fmt.Println()
		fmt.Println("Password changed successfully.")
		fmt.Println("All existing sessions have been invalidated.")
		fmt.Println()
		return nil
	},
}

// onlyagents auth status
// Shows whether auth.yaml exists and who the configured user is.
var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show auth configuration status",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := dataDir()
		username, err := auth.GetUsername(dir)
		if err != nil {
			fmt.Println("Auth not configured. Run `onlyagents server start` to initialise.")
			return nil
		}
		fmt.Printf("Auth configured.\n")
		fmt.Printf("Username : %s\n", username)
		fmt.Printf("Data dir : %s\n", dir)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authResetCmd)
	authCmd.AddCommand(authSetPasswordCmd)
	authCmd.AddCommand(authStatusCmd)
}
