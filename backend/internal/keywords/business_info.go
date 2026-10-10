package keywords

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "math"
 "strings"

 "github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

const businessInfoPath="/v3/business_data/google/my_business_info/live"
type BusinessInfoInput struct { Keyword,Coordinate string;LocationCode int;LanguageCode string }
type BusinessInfoProvider interface { BusinessInfo(context.Context,string,BusinessInfoInput)(map[string]any,error) }
func (p DataForSEOProvider) BusinessInfo(ctx context.Context,org string,in BusinessInfoInput)(map[string]any,error){
 body:=map[string]any{"keyword":in.Keyword,"language_code":in.LanguageCode}
 if in.Coordinate!=""{body["location_coordinate"]=in.Coordinate}else{body["location_code"]=in.LocationCode}
 results,err:=p.post(ctx,org,businessInfoPath,body)
 if taskErr,ok:=errors.AsType[*dataforseo.TaskError](err);ok&&taskErr.StatusCode==40501{return nil,nil}
 if err!=nil{return nil,err};if len(results)==0{return nil,nil}
 var entry struct{Items []map[string]any `json:"items"`;CheckURL string `json:"check_url"`}
 if err=json.Unmarshal(results[0],&entry);err!=nil{return nil,err}
 if len(entry.Items)==0{return nil,nil};profile:=entry.Items[0];if profile["check_url"]==nil&&entry.CheckURL!=""{profile["check_url"]=entry.CheckURL};return profile,nil
}
func FormatBusinessDataCoordinate(lat,lon,radiusKm float64)string{
 f:=func(v float64)string{return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.7f",v),"0"),".")}
 meters:=int(math.Round(radiusKm*1000));if meters<200{meters=200};if meters>199999{meters=199999}
 return fmt.Sprintf("%s,%s,%d",f(lat),f(lon),meters)
}
