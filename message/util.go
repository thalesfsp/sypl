// Copyright 2021 The sypl Authors. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package message

import (
	"crypto/sha1"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/thalesfsp/sypl/shared"
)

// generateUUID generates UUIDv4 for message ID.
func generateUUID() string {
	id, err := uuid.NewRandom()
	if err != nil {
		log.Println(shared.ErrorPrefix, "generateUUID: Failed to generate UUID for message", err)
	}

	return id.String()
}

// generateID generates MD5 hash (content-based) for message ID. Good to be used
// to avoid duplicated messages.
func generateID(ct string) string {
	return fmt.Sprintf("%x\n", sha1.Sum([]byte(strings.Trim(ct, "\f\t\r\n "))))
}
