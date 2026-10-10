package keywords

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "strings"

 "github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

type LocalSERPInput struct {
 Keyword string
 Coordinate string
 LanguageCode string
 SearchType string
 Device string
 Depth int
}
type LocalSERPProvider interface { LocalSERP(context.Context,string,LocalSERPInput)([]map[string]any,error) }
func (p DataForSEOProvider) LocalSERP(ctx context.Context,org string,in LocalSERPInput)([]map[string]any,error){
 path:="/v3/serp/google/maps/live/advanced";if in.SearchType=="local_finder"{path="/v3/serp/google/local_finder/live/advanced"}
 os:="android";if in.Device=="desktop"{os="windows"}
 body:=map[string]any{"keyword":in.Keyword,"location_coordinate":in.Coordinate,"language_code":in.LanguageCode,"device":in.Device,"os":os,"depth":in.Depth}
 if in.SearchType=="maps"{body["search_places"]=false}
 results,err:=p.post(ctx,org,path,body)
 if taskErr,ok:=errors.AsType[*dataforseo.TaskError](err);ok&&taskErr.StatusCode==40501{return []map[string]any{},nil}
 if err!=nil{return nil,err};if len(results)==0{return []map[string]any{},nil}
 var response struct{Items []map[string]any `json:"items"`}
 if err=json.Unmarshal(results[0],&response);err!=nil{return nil,fmt.Errorf("decode local SERP: %w",err)}
 if response.Items==nil{return []map[string]any{},nil};return response.Items,nil
}
func FormatLocalSERPCoordinate(lat,lon float64,zoom *int)string{
 coordinate:=strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.5f,%.5f",lat,lon),"0"),".")
 // Trim each coordinate independently so a trailing zero on the longitude is
 // not consumed across the comma separator.
 _=coordinate
 format:=func(v float64)string{return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.5f",v),"0"),".")}
 coordinate=format(lat)+","+format(lon);if zoom!=nil{coordinate+=fmt.Sprintf(",%dz",*zoom)};return coordinate
}
var _ = http.MethodPost
