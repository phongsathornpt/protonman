package domain_test

import (
	"encoding/base64"
	"testing"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

func TestContentPartValidation(t *testing.T) {
	validImage := domain.ContentPart{
		Type:     domain.ContentPartImage,
		MIMEType: "image/png",
		Data:     base64.StdEncoding.EncodeToString([]byte("image")),
	}
	if err := validImage.Validate(); err != nil {
		t.Fatalf("valid image part failed: %v", err)
	}

	for name, part := range map[string]domain.ContentPart{
		"unknown type": {Type: "audio"},
		"missing mime": {Type: domain.ContentPartImage, Data: validImage.Data},
		"missing data": {Type: domain.ContentPartImage, MIMEType: "image/png"},
		"bad base64":  {Type: domain.ContentPartImage, MIMEType: "image/png", Data: "%%%"},
		"mixed text":  {Type: domain.ContentPartText, Text: "hello", Data: validImage.Data},
	} {
		t.Run(name, func(t *testing.T) {
			if err := part.Validate(); err == nil {
				t.Fatalf("Validate() accepted %+v", part)
			}
		})
	}
}

func TestMessageValidationIncludesContentParts(t *testing.T) {
	message := domain.Message{
		Role: domain.RoleUser,
		Parts: []domain.ContentPart{{
			Type: domain.ContentPartImage, MIMEType: "image/png", Data: "not-base64",
		}},
	}
	if err := message.Validate(); err == nil {
		t.Fatal("Message.Validate() accepted invalid image part")
	}
}
