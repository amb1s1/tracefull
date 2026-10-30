package cumulus

import "slices"

type linkInfo struct {
	Kind      string `json:"info_kind"`
	SlaveKind string `json:"info_slave_kind"`
	Data      struct {
		Table *uint32 `json:"table"`
	} `json:"info_data"`
	SlaveData struct {
		Table *uint32 `json:"table"`
	} `json:"info_slave_data"`
}

func vrfTable(row link) *uint32 {
	info := row.LinkInfo
	if info.Kind == "vrf" && row.Master == "" && info.SlaveKind == "" && info.SlaveData.Table == nil && info.Data.Table != nil && *info.Data.Table > 0 {
		return info.Data.Table
	}
	return nil
}

// detailedContext requires agreement between the member and its typed VRF in
// the same snapshot. A master name alone can also describe a bridge or bond.
func detailedContext(row link, links map[string]link) *string {
	info := row.LinkInfo
	if info.Kind == "vrf" {
		if vrfTable(row) != nil {
			return &row.Name
		}
		return nil
	}
	if row.Master != "" {
		parent := links[row.Master]
		table := vrfTable(parent)
		if info.Data.Table == nil && info.SlaveKind == "vrf" && info.SlaveData.Table != nil && table != nil && *info.SlaveData.Table == *table {
			return &parent.Name
		}
		return nil
	}
	if info.SlaveKind == "" && info.SlaveData.Table == nil && info.Data.Table == nil && row.Flags != nil && !slices.Contains(row.Flags, "MASTER") {
		context := "default"
		return &context
	}
	return nil
}
