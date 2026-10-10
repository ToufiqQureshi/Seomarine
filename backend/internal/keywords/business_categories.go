package keywords

import (
 "context"
 "encoding/json"
 "errors"
 "net/http"
 "strings"

 "github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

const businessCategoriesPath="/v3/business_data/business_listings/categories"
type BusinessCategory struct { Category string `json:"category"`; BusinessCount *float64 `json:"businessCount"` }
type BusinessCategoriesProvider interface { BusinessCategories(context.Context)([]BusinessCategory,error) }

// BusinessCategories reads DataForSEO's free category index without metering.
func (p DataForSEOProvider) BusinessCategories(ctx context.Context)([]BusinessCategory,error){
 if p.Client==nil{return nil,errors.New("DataForSEO client is not configured")}
 raw,err:=p.Client.DoUnmetered(ctx,http.MethodGet,businessCategoriesPath,nil,true);if err!=nil{return nil,err}
 entries,err:=dataforseo.Results(raw);if err!=nil{return nil,err}
 rows:=make([]BusinessCategory,0,len(entries))
 for _,entry:=range entries{var wire struct{Category string `json:"category_name"`;BusinessCount *float64 `json:"business_count"`};if err:=json.Unmarshal(entry,&wire);err!=nil{return nil,err};if strings.TrimSpace(wire.Category)!=""{rows=append(rows,BusinessCategory{Category:wire.Category,BusinessCount:wire.BusinessCount})}}
 return rows,nil
}
