package easytier

import (
	"testing"
)

func TestEncodeGenerateCredentialRequest(t *testing.T) {
	body, err := encodeGenerateCredentialRequest("", CredentialRequest{
		Groups: []string{"admin", "guest"}, AllowRelay: true, TTLSeconds: 3600, Reusable: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	fields, err := wireFields(body)
	if err != nil {
		t.Fatal(err)
	}
	var groups []string
	var allowRelay, ttl, reusable uint64
	var hasReusable bool
	for _, field := range fields {
		switch field.number {
		case 1:
			groups = append(groups, string(field.bytes))
		case 2:
			allowRelay = field.value
		case 4:
			ttl = field.value
		case 6:
			t.Error("local request must not carry an instance selector")
		case 7:
			hasReusable = true
			reusable = field.value
		default:
			t.Errorf("unexpected field %d", field.number)
		}
	}
	if len(groups) != 2 || groups[0] != "admin" || groups[1] != "guest" {
		t.Errorf("groups: %v", groups)
	}
	if allowRelay != 1 || ttl != 3600 || !hasReusable || reusable != 0 {
		t.Errorf("allowRelay=%d ttl=%d reusable=%d (set %v)", allowRelay, ttl, reusable, hasReusable)
	}

	// An instance-scoped request carries InstanceIdentifier.id (a UUID).
	body, err = encodeGenerateCredentialRequest("12345678-1234-1234-1234-123456789abc", CredentialRequest{TTLSeconds: 60, Reusable: true})
	if err != nil {
		t.Fatal(err)
	}
	fields, err = wireFields(body)
	if err != nil {
		t.Fatal(err)
	}
	var selector []byte
	for _, field := range fields {
		if field.number == 6 {
			selector = field.bytes
		}
		if field.number == 7 && field.value != 1 {
			t.Error("reusable = true must encode as 1")
		}
	}
	if len(selector) == 0 {
		t.Fatal("remote request lost its instance selector")
	}
	inner, err := wireFields(selector)
	if err != nil || len(inner) != 1 || inner[0].number != 1 {
		t.Fatalf("selector: %+v %v", inner, err)
	}
	id, err := decodeUUID(inner[0].bytes)
	if err != nil || id != "12345678-1234-1234-1234-123456789abc" {
		t.Fatalf("selector decodes to %q, %v", id, err)
	}

	if _, err := encodeGenerateCredentialRequest("", CredentialRequest{}); err == nil {
		t.Error("ttl 0 must be rejected before the wire")
	}
	if _, err := encodeGenerateCredentialRequest("not-a-uuid", CredentialRequest{TTLSeconds: 1}); err == nil {
		t.Error("bad instance id must be rejected")
	}
}

func TestCredentialInfoDecoding(t *testing.T) {
	// ListCredentialsResponse{credentials = 1, repeated CredentialInfo}.
	var entry []byte
	entry = wireBytes(entry, 1, []byte("cred-1"))
	entry = wireBytes(entry, 2, []byte("admin"))
	entry = wireInt(entry, 3, 1)
	entry = wireInt(entry, 4, 1735689600)
	entry = wireBytes(entry, 5, []byte("10.0.0.0/24"))
	entry = wireInt(entry, 6, 0)
	response := wireBytes(nil, 1, entry)

	credentials, err := decodeCredentialList(response)
	if err != nil {
		t.Fatal(err)
	}
	if len(credentials) != 1 {
		t.Fatalf("credentials: %v", credentials)
	}
	got := credentials[0]
	if got.ID != "cred-1" || len(got.Groups) != 1 || got.Groups[0] != "admin" ||
		!got.AllowRelay || got.ExpiryUnix != 1735689600 || got.Reusable ||
		len(got.ProxyCIDRs) != 1 || got.ProxyCIDRs[0] != "10.0.0.0/24" {
		t.Fatalf("decoded: %+v", got)
	}
}
