package application

import (
	"errors"
	"testing"

	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
)

func TestNormalizeInvoiceSubmission(t *testing.T) {
	// 不开票时忽略所有开票字段。
	empty, err := normalizeInvoiceSubmission(orderdomain.InvoiceTypeNone, "  ", "", "")
	if err != nil {
		t.Fatalf("no invoice should not validate fields: %v", err)
	}
	if empty.Title != "" || empty.TaxNo != "" || empty.Email != "" {
		t.Fatalf("no invoice should drop fields: %+v", empty)
	}

	valid, err := normalizeInvoiceSubmission(orderdomain.InvoiceTypeGeneral, " 示例科技 ", " 91310000abc ", " BILL@Example.COM ")
	if err != nil {
		t.Fatalf("valid invoice rejected: %v", err)
	}
	if valid.Title != "示例科技" || valid.TaxNo != "91310000ABC" || valid.Email != "bill@example.com" {
		t.Fatalf("invoice fields not normalized: %+v", valid)
	}

	for _, tc := range []struct {
		name                string
		title, taxNo, email string
		wantErr             error
	}{
		{"missing_title", "", "91310000ABC", "bill@example.com", ErrInvoiceInfoRequired},
		{"missing_tax_no", "示例科技", "", "bill@example.com", ErrInvoiceInfoRequired},
		{"missing_email", "示例科技", "91310000ABC", "", ErrInvoiceInfoRequired},
		{"invalid_email", "示例科技", "91310000ABC", "not-an-email", ErrInvalidEmail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := normalizeInvoiceSubmission(orderdomain.InvoiceTypeSpecial, tc.title, tc.taxNo, tc.email)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}
