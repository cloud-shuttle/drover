package main

import (
	"fmt"
	"time"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/spf13/cobra"
)

// operatorCmd manages operators for multiplayer collaboration
func operatorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operator",
		Short: "Manage operators for multiplayer collaboration",
		Long: `Manage operators (users) in the Drover system.

Operators can be created with API keys for authentication in multiplayer scenarios.`,
	}

	cmd.AddCommand(
		operatorCreateCmd(),
		operatorListCmd(),
		operatorDeleteCmd(),
		operatorLoginCmd(),
	)

	return cmd
}

// operatorCreateCmd creates a new operator
func operatorCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new operator",
		Long: `Create a new operator with a generated API key.

The API key can be used for authentication in multiplayer scenarios.

Example:
  drover operator create alice`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			name := args[0]

			// Check if operator already exists
			_, err = store.GetOperatorByName(name)
			if err == nil {
				return fmt.Errorf("operator '%s' already exists", name)
			}

			// Create operator
			op, err := store.CreateOperator(name)
			if err != nil {
				return fmt.Errorf("creating operator: %w", err)
			}

			fmt.Printf("✅ Operator created successfully!\n\n")
			fmt.Printf("Name: %s\n", op.Name)
			fmt.Printf("API Key: %s\n\n", op.APIKey)
			fmt.Printf("Save this API key securely. You'll need it to authenticate as this operator.\n")
			fmt.Printf("Use it with: export DROVER_API_KEY=%s\n", op.APIKey)

			return nil
		},
	}
}

// operatorListCmd lists all operators
func operatorListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all operators",
		Long: `List all operators in the system.

Example:
  drover operator list`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			operators, err := store.ListOperators()
			if err != nil {
				return fmt.Errorf("listing operators: %w", err)
			}

			if len(operators) == 0 {
				fmt.Println("No operators found. Create one with: drover operator create <name>")
				return nil
			}

			fmt.Printf("Operators (%d):\n\n", len(operators))
			for _, op := range operators {
				fmt.Printf("  • %s\n", op.Name)
				if op.LastActive != nil {
					lastActive := time.Unix(*op.LastActive, 0)
					fmt.Printf("    Last active: %s\n", lastActive.Format(time.RFC1123))
				} else {
					fmt.Printf("    Last active: never\n")
				}
			}

			return nil
		},
	}
}

// operatorDeleteCmd deletes an operator
func operatorDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an operator",
		Long: `Delete an operator from the system.

Warning: This action cannot be undone.

Example:
  drover operator delete alice`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			name := args[0]

			// Check if operator exists
			_, err = store.GetOperatorByName(name)
			if err != nil {
				return fmt.Errorf("operator '%s' not found", name)
			}

			// Delete operator
			if err := store.DeleteOperator(name); err != nil {
				return fmt.Errorf("deleting operator: %w", err)
			}

			fmt.Printf("✅ Operator '%s' deleted successfully\n", name)

			return nil
		},
	}
}

// operatorLoginCmd authenticates as an operator using API key
func operatorLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login <name>",
		Short: "Login as an operator (sets default operator)",
		Long: `Set the default operator for the current session.

This saves the operator name to the configuration file, so you don't need to
specify it for every command.

Example:
  drover operator login alice`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			// Update config with operator
			if err := config.SetOperator(name); err != nil {
				return fmt.Errorf("setting operator: %w", err)
			}

			fmt.Printf("✅ Logged in as '%s'\n", name)

			return nil
		},
	}
}
