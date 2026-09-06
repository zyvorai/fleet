// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package auth

import "testing"

func TestPasswordHashAndVerify(t *testing.T) {
	h, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if h == "correct-horse-battery" {
		t.Fatal("password was not hashed")
	}
	if !VerifyPassword(h, "correct-horse-battery") {
		t.Fatal("expected valid password")
	}
	if VerifyPassword(h, "wrong-password") {
		t.Fatal("wrong password verified")
	}
	if VerifyPassword("garbage", "correct-horse-battery") {
		t.Fatal("malformed hash verified")
	}
}

func TestRejectShortPassword(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("expected short password rejection")
	}
}
