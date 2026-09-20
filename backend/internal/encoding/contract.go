package encoding

import (
	"context"
	"time"
)

const ContractVersion = "v1"

type Job struct {
	ContractVersion string `json:"contract_version"`
	VideoID         string `json:"video_id"`
	OwnerID         string `json:"owner_id"`
	SourceObjectKey string `json:"source_object_key"`
	OutputPrefix    string `json:"output_prefix"`
	PublicKey       string `json:"public_key"`
	SharedKeyID     string `json:"shared_key_id"`
	KeyVersion      string `json:"key_version"`
}

type EncryptionMetadata struct {
	Algorithm        string `json:"algorithm"`
	ChunkSize        int    `json:"chunk_size"`
	KeyVersion       string `json:"key_version"`
	Nonce            string `json:"nonce"`
	EncryptedDataKey string `json:"encrypted_data_key"`
	SharedKeyID      string `json:"shared_key_id"`
}

type Artifact struct {
	ObjectKey  string             `json:"object_key"`
	Encryption EncryptionMetadata `json:"encryption"`
}

type Event struct {
	ContractVersion string              `json:"contract_version"`
	VideoID         string              `json:"video_id"`
	OwnerID         string              `json:"owner_id"`
	SourceObjectKey string              `json:"source_object_key"`
	Status          string              `json:"status"`
	Progress        float64             `json:"progress"`
	Manifest        Artifact            `json:"manifest"`
	Artifacts       map[string]Artifact `json:"artifacts"`
	Error           string              `json:"error,omitempty"`
	OccurredAt      time.Time           `json:"occurred_at"`
}

type JobDispatcher interface {
	PublishJob(context.Context, Job) error
}
