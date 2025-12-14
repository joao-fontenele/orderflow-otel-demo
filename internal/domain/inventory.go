package domain

type StockLevel struct {
	ItemID    string `json:"item_id"`
	Available int    `json:"available"`
	Reserved  int    `json:"reserved"`
}
