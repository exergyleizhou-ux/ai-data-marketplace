package quality

import "testing"

func schemaColumns(c Check) map[string]columnProfile {
	out := map[string]columnProfile{}
	if cols, ok := c.Report["columns"].([]columnProfile); ok {
		for _, p := range cols {
			out[p.Name] = p
		}
	}
	return out
}

func TestSchemaProfilesColumns(t *testing.T) {
	csv := "id,price,city,flag\n" +
		"1,9.5,北京,true\n" +
		"2,10.5,上海,false\n" +
		"3,11.5,北京,true\n" +
		",,广州,\n"
	c := Schema([]byte(csv), "text/csv")
	if applicable, _ := c.Report["applicable"].(bool); !applicable {
		t.Fatalf("CSV should be applicable: %v", c.Report)
	}
	if c.Report["column_count"].(int) != 4 {
		t.Errorf("column_count = %v, want 4", c.Report["column_count"])
	}
	cols := schemaColumns(c)

	id := cols["id"]
	if id.Type != "integer" || id.NonNull != 3 || id.Null != 1 {
		t.Errorf("id = %+v, want integer non_null=3 null=1", id)
	}
	if id.Min == nil || *id.Min != 1 || id.Max == nil || *id.Max != 3 || id.Mean == nil || *id.Mean != 2 {
		t.Errorf("id stats = min %v max %v mean %v, want 1/3/2", id.Min, id.Max, id.Mean)
	}

	price := cols["price"]
	if price.Type != "number" || price.Mean == nil || *price.Mean != 10.5 {
		t.Errorf("price = %+v, want number mean 10.5", price)
	}

	city := cols["city"]
	if city.Type != "string" || city.Distinct != 3 {
		t.Errorf("city = %+v, want string distinct 3", city)
	}
	if len(city.Samples) == 0 {
		t.Errorf("string column should keep samples, got %+v", city.Samples)
	}

	flag := cols["flag"]
	if flag.Type != "boolean" || flag.NonNull != 3 {
		t.Errorf("flag = %+v, want boolean non_null=3", flag)
	}
}

func TestSchemaTSVAndNonTabular(t *testing.T) {
	tsv := "a\tb\n1\tx\n2\ty\n"
	if c := Schema([]byte(tsv), "text/tab-separated-values"); c.Report["applicable"] != true {
		t.Errorf("TSV should be applicable, got %v", c.Report)
	}
	if c := Schema([]byte(`{"a":1}`), "application/json"); c.Report["applicable"] != false {
		t.Errorf("JSON should be non-applicable, got %v", c.Report)
	}
	// Result is always pass (descriptive, never a gate).
	if c := Schema([]byte("x\n"), "text/plain"); c.Result != ResultPass {
		t.Errorf("schema must never fail, got %s", c.Result)
	}
}
