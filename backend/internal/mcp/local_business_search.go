package mcp

import (
 "context"
 "encoding/json"
 "fmt"
 "strings"

 "github.com/toufiqqureshi/seomarine/backend/internal/keywords"
)

var localBusinessFields=[]string{"title","description","category","additional_categories","address","phone","url","domain","rating","is_claimed","cid","place_id","latitude","longitude","total_photos","check_url"}
func searchLocalBusinessesTool()*tool{return &tool{Name:"search_local_businesses",Title:"Search local businesses",Description:"Search local business listings near a coordinate with optional rating, review count, and claimed-status filters. Charges credits.",
 InputSchema:json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"query":{"type":"string","minLength":1,"maxLength":200},"near":{"type":"object","properties":{"latitude":{"type":"number","minimum":-90,"maximum":90},"longitude":{"type":"number","minimum":-180,"maximum":180},"radiusKm":{"type":"number","minimum":1,"maximum":100000}},"required":["latitude","longitude","radiusKm"],"additionalProperties":false},"categories":{"type":"array","items":{"type":"string","minLength":1,"maxLength":120},"minItems":1,"maxItems":10},"minRating":{"type":"number","minimum":1,"maximum":5},"minReviews":{"type":"integer","minimum":0},"isClaimed":{"type":"boolean"},"sortBy":{"type":"string","enum":["relevance","rating","reviews"]},"limit":{"type":"integer","minimum":1,"maximum":50},"offset":{"type":"integer","minimum":0,"maximum":1000}},"required":["projectId","near"],"additionalProperties":false}`),
 OutputSchema:json.RawMessage(`{"type":"object","properties":{"businesses":{"type":"array","items":{"type":"object"}}},"additionalProperties":true}`),Annotations:map[string]any{"readOnlyHint":false,"openWorldHint":true,"destructiveHint":false},Handler:handleSearchLocalBusinesses}}
func handleSearchLocalBusinesses(ctx context.Context,raw json.RawMessage,env *callEnv)(*callResult,error){
 var a struct{ProjectID string `json:"projectId"`;Query string `json:"query"`;Near struct{Latitude *float64 `json:"latitude"`;Longitude *float64 `json:"longitude"`;RadiusKm *float64 `json:"radiusKm"`} `json:"near"`;Categories []string `json:"categories"`;MinRating *float64 `json:"minRating"`;MinReviews *int `json:"minReviews"`;IsClaimed *bool `json:"isClaimed"`;SortBy string `json:"sortBy"`;Limit int `json:"limit"`;Offset int `json:"offset"`}
 if e:=json.Unmarshal(raw,&a);e!=nil||strings.TrimSpace(a.ProjectID)==""{return nil,newAppErrorf("VALIDATION_ERROR","projectId is required")}
 if a.Query!=""&&(len([]rune(a.Query))>200){return nil,newAppErrorf("VALIDATION_ERROR","query must be at most 200 characters")}
 if a.Near.Latitude==nil||a.Near.Longitude==nil||a.Near.RadiusKm==nil||*a.Near.Latitude < -90||*a.Near.Latitude>90||*a.Near.Longitude < -180||*a.Near.Longitude>180||*a.Near.RadiusKm<1||*a.Near.RadiusKm>100000{return nil,newAppErrorf("VALIDATION_ERROR","near must include valid latitude, longitude, and radiusKm")}
 if a.Categories!=nil&&(len(a.Categories)<1||len(a.Categories)>10){return nil,newAppErrorf("VALIDATION_ERROR","categories must contain 1 to 10 values")};for _,c:=range a.Categories{if strings.TrimSpace(c)==""||len([]rune(c))>120{return nil,newAppErrorf("VALIDATION_ERROR","each category must contain 1 to 120 characters")}}
 if a.MinRating!=nil&&(*a.MinRating<1||*a.MinRating>5){return nil,newAppErrorf("VALIDATION_ERROR","minRating must be from 1 to 5")};if a.MinReviews!=nil&&*a.MinReviews<0{return nil,newAppErrorf("VALIDATION_ERROR","minReviews must be non-negative")}
 limit:=a.Limit;if limit==0{limit=20};if limit<1||limit>50{return nil,newAppErrorf("VALIDATION_ERROR","limit must be from 1 to 50")};if a.Offset<0||a.Offset>1000{return nil,newAppErrorf("VALIDATION_ERROR","offset must be from 0 to 1000")}
 switch a.SortBy{case "","relevance","rating","reviews":default:return nil,newAppErrorf("VALIDATION_ERROR","sortBy is invalid")}
 access,e:=env.h.authorizeProject(ctx,env.auth,a.ProjectID);if e!=nil{return nil,e};if env.deps.KeywordResearch==nil{return nil,newAppErrorf("SERVICE_UNAVAILABLE","Local business search is not configured.")}
 provider,ok:=env.deps.KeywordResearch.Data.(keywords.BusinessListingsSearchProvider);if !ok{return nil,newAppErrorf("SERVICE_UNAVAILABLE","Local business search is not configured.")}
 var filters [][]any;if a.MinRating!=nil{filters=append(filters,[]any{"rating.value",">=",*a.MinRating})};if a.MinReviews!=nil{filters=append(filters,[]any{"rating.votes_count",">=",*a.MinReviews})}
 var order []string;if a.SortBy=="rating"{order=[]string{"rating.value,desc"}}else if a.SortBy=="reviews"{order=[]string{"rating.votes_count,desc"}}
 rows,e:=provider.BusinessListingsSearch(ctx,access.Auth.OrganizationID,keywords.BusinessListingsSearchInput{Categories:a.Categories,Title:strings.TrimSpace(a.Query),Coordinate:keywords.FormatBusinessListingsCoordinate(*a.Near.Latitude,*a.Near.Longitude,*a.Near.RadiusKm),IsClaimed:a.IsClaimed,Filters:filters,OrderBy:order,Limit:limit,Offset:a.Offset})
 if e!=nil{env.deps.Logger.ErrorContext(ctx,"MCP local business search","project_id",access.Project.ID,"err",e);return nil,newAppErrorf("INTERNAL_ERROR","Unable to search local businesses.")}
 businesses:=make([]map[string]any,0,len(rows));for _,row:=range rows{businesses=append(businesses,pickMCPFields(row,localBusinessFields))}
 header:=fmt.Sprintf("Found %d local business rows%s.",len(businesses),func()string{if a.Query!=""{return " for "+a.Query};return ""}());lines:=[]string{header};for _,b:=range businesses{rating,_:=b["rating"].(map[string]any);lines=append(lines,fmt.Sprintf("%v | %v | %v | %v | %v | %v",b["title"],b["category"],rating["value"],rating["votes_count"],b["phone"],b["address"]))}
 return mcpResponse(strings.Join(lines,"\n"),map[string]any{"businesses":businesses},metaFields{ProjectID:access.Project.ID,URL:buildDashboardURL(env.auth.BaseURL,"/p/"+access.Project.ID,nil)}),nil
}
