package desk

import "testing"

func TestModelIsWhatPeopleKnowTheirComputerBy(t *testing.T) {
	for _, c := range []struct{ maker, product, version, want string }{
		{"Micro-Star International Co., Ltd.", "MS-7D48", "1.0", "MSI MS-7D48"},
		{"Micro-Star International Co., Ltd.", "Modern 15 B12M", "REV:1.0", "MSI Modern 15 B12M"},
		{"ASUSTeK COMPUTER INC.", "ROG Strix G513QM_G513QM", "1.0", "ASUS ROG Strix G513QM"},
		{"ASUSTeK COMPUTER INC.", "VivoBook_ASUSLaptop X512DA_X512DA", "1.0", "ASUS VivoBook ASUSLaptop X512DA"},
		{"ASUSTeK COMPUTER INC.", "ASUS TUF Gaming A15 FA506IH", "1.0", "ASUS TUF Gaming A15 FA506IH"},
		{"System manufacturer", "System Product Name", "System Version", "PC"},
		{"To Be Filled By O.E.M.", "To Be Filled By O.E.M.", "To Be Filled By O.E.M.", "PC"},
		{"Gigabyte Technology Co., Ltd.", "B550 AORUS ELITE", "Default string", "Gigabyte B550 AORUS ELITE"},
		{"LENOVO", "20XW0026RT", "ThinkPad X1 Carbon Gen 9", "Lenovo ThinkPad X1 Carbon Gen 9"},
		{"LENOVO", "82JW", "Lenovo Legion 5 15ACH6H", "Lenovo Legion 5 15ACH6H"},
		{"HP", "HP Pavilion Laptop 15-eh1xxx", "Type1ProductConfigId", "HP Pavilion Laptop 15-eh1xxx"},
		{"Hewlett-Packard", "HP EliteBook 840 G3", "", "HP EliteBook 840 G3"},
		{"Dell Inc.", "XPS 15 9520", "", "Dell XPS 15 9520"},
		{"Acer", "Aspire A515-56", "V1.12", "Acer Aspire A515-56"},
		{"TIMI", "RedmiBook Pro 15", "", "Xiaomi RedmiBook Pro 15"},
		{"HUAWEI", "MateBook D 15", "M1010", "Huawei MateBook D 15"},
		{"Microsoft Corporation", "Surface Laptop 5", "", "Microsoft Surface Laptop 5"},
		{"Some Vendor GmbH", "Box 1", "", "Some Vendor Box 1"},
		{"  ", "  Mini   PC  ", "", "Mini PC"},
		{"", "", "", "PC"},
		{"Vendor", "", "", "Vendor"},
	} {
		if got := Model(c.maker, c.product, c.version); got != c.want {
			t.Errorf("%q %q %q: %q, want %q", c.maker, c.product, c.version, got, c.want)
		}
	}
	long := Model("Vendor", "a very long product name that goes on and on and on and on and on and on", "")
	if n := len([]rune(long)); n > 64 || n == 0 {
		t.Fatalf("%d runes: %q", n, long)
	}
}
