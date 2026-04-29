package theorydb

import "fmt"

type transactionKeyItem struct {
	tableName string

	PK any `theorydb:"pk,attr:PK"`
	SK any `theorydb:"sk,attr:SK,omitempty"`
}

func newTransactionKeyItem(tableName string, key map[string]any) (*transactionKeyItem, error) {
	if tableName == "" {
		return nil, fmt.Errorf("key operation requires tableName")
	}
	if key == nil {
		return nil, fmt.Errorf("key operation requires key")
	}

	pk, ok := key["PK"]
	if !ok {
		pk, ok = key["pk"]
	}
	if !ok || pk == nil {
		return nil, fmt.Errorf("key operation requires PK")
	}

	sk := key["SK"]
	if sk == nil {
		sk = key["sk"]
	}

	return &transactionKeyItem{
		tableName: tableName,
		PK:        pk,
		SK:        sk,
	}, nil
}

func (i *transactionKeyItem) TableName() string {
	return i.tableName
}
