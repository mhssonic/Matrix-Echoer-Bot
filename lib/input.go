package lib

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gotd/td/tg"
)

// Read verification code from terminal
func readCode(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	fmt.Print("Enter the code Telegram sent you: ")
	reader := bufio.NewReader(os.Stdin)
	code, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(code), nil
}
