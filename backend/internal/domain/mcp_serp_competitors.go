package domain

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "regexp"
 "strings"
)

const serpCompetitorsPath="/v3/dataforseo_labs/google/serp_competitors/live"
var competitorDomainPattern=regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$`)

type MCPSerpCompetitorInput struct {
 Keywords []string
 LocationCode int
 LanguageCode string
 ItemTypes []string
 IncludeSubdomains *bool
 Limit int
 Offset int
}
type MCPSerpCompetitor struct {
 Domain *string `json:"domain"`
 AvgPosition *float64 `json:"avg_position"`
 MedianPosition *float64 `json:"median_position"`
 Visibility *float64 `json:"visibility"`
 ETV *float64 `json:"etv"`
 KeywordsCount *float64 `json:"keywords_count"`
}
func (s *Service) SerpCompetitorsForMCP(ctx context.Context,org string,in MCPSerpCompetitorInput)([]MCPSerpCompetitor,error){
 if len(in.Keywords)<1||len(in.Keywords)>100{return nil,badRequest("keywords must contain between 1 and 100 values.")}
 for _,k:=range in.Keywords{if strings.TrimSpace(k)==""||len([]rune(k))>120{return nil,badRequest("Each keyword must contain 1 to 120 characters.")}}
 if in.Limit==0{in.Limit=50};if in.Limit<1||in.Limit>100{return nil,badRequest("limit must be between 1 and 100.")}
 if in.Offset<0||in.Offset>1000{return nil,badRequest("offset must be between 0 and 1000.")}
 types:=in.ItemTypes;if len(types)==0{types=[]string{"organic","local_pack"}};if len(types)>4{return nil,badRequest("resultTypes must contain between 1 and 4 values.")}
 for _,v:=range types{switch v{case "organic","paid","featured_snippet","local_pack":default:return nil,badRequest("resultTypes contains an unsupported value.")}}
 body:=map[string]any{"keywords":in.Keywords,"location_code":in.LocationCode,"language_code":in.LanguageCode,"item_types":types,"limit":in.Limit}
 if in.IncludeSubdomains!=nil{body["include_subdomains"]=*in.IncludeSubdomains};if in.Offset>0{body["offset"]=in.Offset}
 results,err:=s.provider.post(ctx,org,serpCompetitorsPath,[]map[string]any{body});if err!=nil{return nil,err};if len(results)==0{return []MCPSerpCompetitor{},nil}
 var response struct{Items []MCPSerpCompetitor `json:"items"`}
 if err=json.Unmarshal(results[0],&response);err!=nil{return nil,fmt.Errorf("decode SERP competitors: %w",err)}
 if response.Items==nil{return []MCPSerpCompetitor{},nil};return response.Items,nil
}
func ValidMCPCompetitorDomain(domain string)bool{
 v:=strings.ToLower(strings.TrimPrefix(strings.TrimSpace(domain),"www."));return competitorDomainPattern.MatchString(v)
}

// IsBadRequest reports service validation errors safe to show to MCP callers.
func IsBadRequest(err error) bool { var target badRequest; return errors.As(err,&target) }
