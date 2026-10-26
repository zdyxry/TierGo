package easytier

// Temporary-credential management over CredentialManageRpc (main only;
// v2.6.4 has no such service, and its rejection surfaces as an RPC error).
// Wire layout follows internal/easytier/schema/api_instance.proto.
import (
	"context"
	"fmt"

	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// Credential is one temporary credential as ListCredentials reports it.
type Credential struct {
	ID         string
	Groups     []string
	AllowRelay bool
	ExpiryUnix int64
	ProxyCIDRs []string
	Reusable   bool
}

// CredentialRequest is what GenerateCredential takes. TTLSeconds must be
// positive; the core rejects 0.
type CredentialRequest struct {
	Groups     []string
	AllowRelay bool
	TTLSeconds int64
	Reusable   bool
}

// GeneratedCredential is the one time the secret is shown.
type GeneratedCredential struct {
	ID     string
	Secret string
}

func GenerateCredential(ctx context.Context, address string, request CredentialRequest) (GeneratedCredential, error) {
	return generateCredential(ctx, address, "", request)
}

// GenerateRemoteCredential targets one instance behind a shared remote
// management endpoint.
func GenerateRemoteCredential(ctx context.Context, address string, network RemoteNetwork, request CredentialRequest) (GeneratedCredential, error) {
	address, err := NormalizeRPCAddress(address)
	if err != nil {
		return GeneratedCredential{}, err
	}
	return generateCredential(ctx, address, network.ID, request)
}

// encodeGenerateCredentialRequest builds GenerateCredentialRequest: groups
// (1), allow_relay (2), ttl_seconds (4), instance (6), reusable (7).
// allowed_proxy_cidrs (3) and credential_id (5) stay unset: the GUI does not
// offer them.
func encodeGenerateCredentialRequest(instanceID string, request CredentialRequest) ([]byte, error) {
	if request.TTLSeconds <= 0 {
		return nil, fmt.Errorf("%s", locale.T("凭据有效期必须大于 0 秒"))
	}
	var body []byte
	for _, group := range request.Groups {
		body = wireBytes(body, 1, []byte(group))
	}
	if request.AllowRelay {
		body = wireInt(body, 2, 1)
	}
	body = wireInt(body, 4, uint64(request.TTLSeconds))
	selector, err := instanceSelector(instanceID)
	if err != nil {
		return nil, err
	}
	if selector != nil {
		body = wireBytes(body, 6, selector)
	}
	return wireInt(body, 7, boolWire(request.Reusable)), nil
}

func generateCredential(ctx context.Context, address, instanceID string, request CredentialRequest) (GeneratedCredential, error) {
	body, err := encodeGenerateCredentialRequest(instanceID, request)
	if err != nil {
		return GeneratedCredential{}, err
	}
	data, err := managementRPC(ctx, address, "CredentialManageRpc", 1, body)
	if err != nil {
		return GeneratedCredential{}, err
	}
	fields, err := wireFields(data)
	if err != nil {
		return GeneratedCredential{}, err
	}
	var generated GeneratedCredential
	for _, field := range fields {
		switch field.number {
		case 1:
			generated.ID = string(field.bytes)
		case 2:
			generated.Secret = string(field.bytes)
		}
	}
	if generated.ID == "" {
		return GeneratedCredential{}, fmt.Errorf("%s", locale.T("Core 未返回凭据 ID"))
	}
	return generated, nil
}

func RevokeCredential(ctx context.Context, address, credentialID string) error {
	return revokeCredential(ctx, address, "", credentialID)
}

// RevokeRemoteCredential targets one instance behind a shared remote
// management endpoint.
func RevokeRemoteCredential(ctx context.Context, address string, network RemoteNetwork, credentialID string) error {
	address, err := NormalizeRPCAddress(address)
	if err != nil {
		return err
	}
	return revokeCredential(ctx, address, network.ID, credentialID)
}

func revokeCredential(ctx context.Context, address, instanceID, credentialID string) error {
	if credentialID == "" {
		return fmt.Errorf("%s", locale.T("凭据 ID 不能为空"))
	}
	body := wireBytes(nil, 1, []byte(credentialID))
	selector, err := instanceSelector(instanceID)
	if err != nil {
		return err
	}
	if selector != nil {
		body = wireBytes(body, 2, selector)
	}
	_, err = managementRPC(ctx, address, "CredentialManageRpc", 2, body)
	return err
}

func ListCredentials(ctx context.Context, address string) ([]Credential, error) {
	return listCredentials(ctx, address, "")
}

// ListRemoteCredentials targets one instance behind a shared remote
// management endpoint.
func ListRemoteCredentials(ctx context.Context, address string, network RemoteNetwork) ([]Credential, error) {
	address, err := NormalizeRPCAddress(address)
	if err != nil {
		return nil, err
	}
	return listCredentials(ctx, address, network.ID)
}

func listCredentials(ctx context.Context, address, instanceID string) ([]Credential, error) {
	var body []byte
	selector, err := instanceSelector(instanceID)
	if err != nil {
		return nil, err
	}
	if selector != nil {
		body = wireBytes(body, 1, selector)
	}
	data, err := managementRPC(ctx, address, "CredentialManageRpc", 3, body)
	if err != nil {
		return nil, err
	}
	return decodeCredentialList(data)
}

// decodeCredentialList reads ListCredentialsResponse{credentials = 1}.
func decodeCredentialList(data []byte) ([]Credential, error) {
	fields, err := wireFields(data)
	if err != nil {
		return nil, err
	}
	var credentials []Credential
	for _, field := range fields {
		if field.number != 1 {
			continue
		}
		values, err := wireFields(field.bytes)
		if err != nil {
			return credentials, err
		}
		credential := Credential{Reusable: true}
		for _, value := range values {
			switch value.number {
			case 1:
				credential.ID = string(value.bytes)
			case 2:
				credential.Groups = append(credential.Groups, string(value.bytes))
			case 3:
				credential.AllowRelay = value.value != 0
			case 4:
				credential.ExpiryUnix = int64(value.value)
			case 5:
				credential.ProxyCIDRs = append(credential.ProxyCIDRs, string(value.bytes))
			case 6:
				credential.Reusable = value.value != 0
			}
		}
		credentials = append(credentials, credential)
	}
	return credentials, nil
}

func boolWire(value bool) uint64 {
	if value {
		return 1
	}
	return 0
}
