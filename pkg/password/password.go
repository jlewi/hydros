package password

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/thanhpk/randstr"
	"golang.org/x/crypto/pbkdf2"
)

type encodePassOptions struct {
	iterations int
	keyLength  int
	salt       string
}

// NewEncodePasswordCmd return the cobra command for encoding the supplied password
func NewEncodePasswordCmd(w io.Writer) *cobra.Command {
	encodeOptions := encodePassOptions{}
	cmd := &cobra.Command{
		Use:   "encode-password",
		Short: "encodes a password via PBKDF2",
		Run: func(cmd *cobra.Command, args []string) {
			if encodeOptions.salt == "" {
				encodeOptions.salt = randstr.Hex(16)
			}

			encodedPassword := encodePasswordToPBKDF2(
				args[0],
				encodeOptions.iterations,
				encodeOptions.keyLength,
				encodeOptions.salt,
			)
			_, err := io.WriteString(w, encodedPassword)
			if err != nil {
				os.Exit(1)
			}
		},
	}

	cmd.Flags().IntVarP(&encodeOptions.iterations, "iterations", "i", 310000, "iterations during encoding")
	cmd.Flags().IntVarP(&encodeOptions.keyLength, "key-length", "k", 32, "length of key used during encoding")
	cmd.Flags().StringVarP(&encodeOptions.salt, "salt", "s", "", "salt to be added during encoding - default to randstr")

	return cmd
}

func encodePasswordToPBKDF2(password string, iterations int, keyLength int, salt string) string {
	encodedPassBytes := pbkdf2.Key([]byte(password), []byte(salt), iterations, keyLength, sha256.New)
	formattedString := fmt.Sprintf("pbkdf2:sha256:%v$%s$%s", iterations, salt, hex.EncodeToString(encodedPassBytes))
	return formattedString
}
