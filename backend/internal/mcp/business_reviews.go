package mcp

import (
 "context"
 "encoding/json"
 "fmt"
 "strings"
 "time"

 "github.com/toufiqqureshi/seomarine/backend/internal/keywords"
)

var businessReviewFields=[]string{"rank_absolute","time_ago","timestamp","rating","review_text","original_review_text","original_language","profile_name","local_guide","reviews_count","photos_count","review_highlights","source","owner_answer","owner_time_ago","owner_timestamp","review_id"}
var businessReviewAnswerFields=[]string{"title","reviews_count","rating","cid","place_id"}
func getBusinessReviewsTool()*tool{return &tool{Name:"get_business_reviews",Title:"Get business reviews",Description:"Collects Google reviews for a business. Returns a resumable task ID while collection runs; resuming does not charge again.",
 InputSchema:json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"businessName":{"type":"string","minLength":1,"maxLength":200},"cid":{"type":"string","minLength":1,"maxLength":64},"placeId":{"type":"string","minLength":1,"maxLength":256},"near":{"type":"object","properties":{"latitude":{"type":"number","minimum":-90,"maximum":90},"longitude":{"type":"number","minimum":-180,"maximum":180},"radiusKm":{"type":"number","minimum":0.2,"maximum":199}},"required":["latitude","longitude"],"additionalProperties":false},"locationCode":{"type":"integer","minimum":1},"languageCode":{"type":"string","minLength":2,"maxLength":8},"depth":{"type":"integer","minimum":10,"maximum":200},"sortBy":{"type":"string","enum":["newest","highest_rating","lowest_rating","relevant"]},"includeOtherSources":{"type":"boolean"},"taskId":{"type":"string","minLength":1,"maxLength":128}},"required":["projectId"],"additionalProperties":false}`),
 OutputSchema:json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","enum":["completed","processing"]},"taskId":{"type":"string"},"reviews":{"type":"array","items":{"type":"object"}},"totals":{"type":["object","null"]}},"required":["status","taskId"],"additionalProperties":true}`),Annotations:map[string]any{"readOnlyHint":false,"openWorldHint":true,"destructiveHint":false},Handler:handleGetBusinessReviews}}
func handleGetBusinessReviews(ctx context.Context,raw json.RawMessage,env *callEnv)(*callResult,error){
 var a struct{ProjectID string `json:"projectId"`;BusinessName *string `json:"businessName"`;CID *string `json:"cid"`;PlaceID *string `json:"placeId"`;Near *struct{Latitude *float64 `json:"latitude"`;Longitude *float64 `json:"longitude"`;RadiusKm *float64 `json:"radiusKm"`} `json:"near"`;LocationCode *int `json:"locationCode"`;LanguageCode string `json:"languageCode"`;Depth int `json:"depth"`;SortBy string `json:"sortBy"`;IncludeOtherSources bool `json:"includeOtherSources"`;TaskID string `json:"taskId"`}
 if e:=json.Unmarshal(raw,&a);e!=nil||strings.TrimSpace(a.ProjectID)==""{return nil,newAppErrorf("VALIDATION_ERROR","projectId is required")}
 endpoint:="reviews";publicID:=""
 if a.TaskID!=""{if len(a.TaskID)>128{return nil,newAppErrorf("VALIDATION_ERROR","taskId is too long")};parts:=strings.SplitN(a.TaskID,":",2);if len(parts)!=2||parts[1]==""||(parts[0]!="google"&&parts[0]!="extended"){return nil,newAppErrorf("VALIDATION_ERROR",`taskId must be formatted as "google:<id>" or "extended:<id>".`)};if parts[0]=="extended"{endpoint="extended_reviews"};publicID=a.TaskID
 }else{
  count:=0;for _,v:=range []*string{a.BusinessName,a.CID,a.PlaceID}{if v!=nil{count++}};if count!=1{return nil,newAppErrorf("VALIDATION_ERROR","Provide exactly one business identifier: businessName, cid, or placeId.")}
 }
 if a.Depth!=0&&(a.Depth<10||a.Depth>200){return nil,newAppErrorf("VALIDATION_ERROR","depth must be from 10 to 200")}
 sortBy:=a.SortBy;if sortBy==""{sortBy="newest"};switch sortBy{case "newest","highest_rating","lowest_rating","relevant":default:return nil,newAppErrorf("VALIDATION_ERROR","sortBy is invalid")}
 if a.Near!=nil&&(a.Near.Latitude==nil||a.Near.Longitude==nil||*a.Near.Latitude < -90||*a.Near.Latitude>90||*a.Near.Longitude < -180||*a.Near.Longitude>180){return nil,newAppErrorf("VALIDATION_ERROR","near coordinates are invalid")}
 if a.Near!=nil&&a.Near.RadiusKm!=nil&&(*a.Near.RadiusKm<0.2||*a.Near.RadiusKm>199){return nil,newAppErrorf("VALIDATION_ERROR","radiusKm must be from 0.2 to 199")}
 if a.LocationCode!=nil&&*a.LocationCode<1{return nil,newAppErrorf("VALIDATION_ERROR","locationCode must be positive")}
 if a.LanguageCode!=""&&(len(strings.TrimSpace(a.LanguageCode))<2||len(strings.TrimSpace(a.LanguageCode))>8){return nil,newAppErrorf("VALIDATION_ERROR","languageCode must be 2 to 8 characters")}
 access,e:=env.h.authorizeProject(ctx,env.auth,a.ProjectID);if e!=nil{return nil,e};if env.deps.KeywordResearch==nil{return nil,newAppErrorf("SERVICE_UNAVAILABLE","Business review collection is not configured.")}
 provider,ok:=env.deps.KeywordResearch.Data.(keywords.BusinessReviewTaskProvider);if !ok{return nil,newAppErrorf("SERVICE_UNAVAILABLE","Business review collection is not configured.")}
 if a.TaskID==""{
  keyword,cid,place:="","","";if a.BusinessName!=nil{if len(*a.BusinessName)>200||strings.TrimSpace(*a.BusinessName)==""{return nil,newAppErrorf("VALIDATION_ERROR","businessName is invalid")};keyword=strings.TrimSpace(*a.BusinessName)}else if a.CID!=nil{if len(*a.CID)>64||strings.TrimSpace(*a.CID)==""{return nil,newAppErrorf("VALIDATION_ERROR","cid is invalid")};cid=strings.TrimSpace(*a.CID)}else{if len(*a.PlaceID)>256||strings.TrimSpace(*a.PlaceID)==""{return nil,newAppErrorf("VALIDATION_ERROR","placeId is invalid")};place=strings.TrimSpace(*a.PlaceID)}
  code:=access.Project.LocationCode;if a.LocationCode!=nil{code=*a.LocationCode};language:=strings.TrimSpace(a.LanguageCode);if language==""{language=access.Project.LanguageCode}
  coordinate:="";if a.Near!=nil{radius:=10.0;if a.Near.RadiusKm!=nil{radius=*a.Near.RadiusKm};coordinate=keywords.FormatBusinessDataCoordinate(*a.Near.Latitude,*a.Near.Longitude,radius)}
  endpoint="reviews";if a.IncludeOtherSources{endpoint="extended_reviews"}
  depth:=a.Depth;if depth==0{depth=20}
  id,err:=provider.StartBusinessReviewTask(ctx,access.Auth.OrganizationID,keywords.BusinessReviewTaskInput{Endpoint:endpoint,Keyword:keyword,CID:cid,PlaceID:place,Coordinate:coordinate,LocationCode:code,LanguageCode:language,Depth:depth,SortBy:sortBy})
  if err!=nil{env.deps.Logger.ErrorContext(ctx,"MCP post business reviews","project_id",access.Project.ID,"err",err);return nil,newAppErrorf("INTERNAL_ERROR","Unable to start Google review collection.")}
  prefix:="google";if endpoint=="extended_reviews"{prefix="extended"};publicID=prefix+":"+id
 }
 taskID:=strings.SplitN(publicID,":",2)[1]
 var result map[string]any;pending:=false
 for attempt:=0;attempt<6;attempt++{
  if attempt>0{timer:=time.NewTimer(4*time.Second);select{case <-ctx.Done():timer.Stop();pending=true;attempt=6;case <-timer.C:}}
  if pending{break}
  pending,result,e=provider.PollBusinessReviewTask(ctx,endpoint,taskID);if e!=nil{env.deps.Logger.ErrorContext(ctx,"MCP collect business reviews","project_id",access.Project.ID,"err",e);return nil,newAppErrorf("INTERNAL_ERROR",e.Error()+` The queued task is still collectable — call again with taskId "`+publicID+`" at no extra cost.`)}
  if !pending{break}
 }
 meta:=metaFields{ProjectID:access.Project.ID,URL:buildDashboardURL(env.auth.BaseURL,"/p/"+access.Project.ID,nil)}
 if pending{return mcpResponse(fmt.Sprintf("Review collection is still running. Call get_business_reviews again with taskId %q in 30-60 seconds — resuming charges no extra credits.",publicID),map[string]any{"status":"processing","taskId":publicID},meta),nil}
 rawRows,_:=result["items"].([]any);reviews:=make([]map[string]any,0,len(rawRows));for _,item:=range rawRows{if row,ok:=item.(map[string]any);ok{reviews=append(reviews,pickMCPFields(row,businessReviewFields))}}
 var totals any
 if result!=nil{totals=pickMCPFields(result,businessReviewAnswerFields)}
 totalText:="";if v,ok:=result["reviews_count"];ok{totalText=fmt.Sprintf(" of %v total",v)}
 header:=fmt.Sprintf("Collected %d reviews%s.",len(reviews),totalText);lines:=[]string{header}
 if len(reviews)==0{lines[0]+=" This profile has no reviews matching the request."}else{lines=append(lines,"# | when | rating | author | source | review | owner replied");for _,r:=range reviews{rating,_:=r["rating"].(map[string]any);source:="Google";if s,ok:=r["source"].(map[string]any);ok&&s["title"]!=nil{source=fmt.Sprint(s["title"])};review:=fmt.Sprint(r["review_text"]);if len([]rune(review))>120{review=string([]rune(review)[:120])+"…"};lines=append(lines,fmt.Sprintf("%v | %v | %v | %v | %s | %s | %t",r["rank_absolute"],r["time_ago"],rating["value"],r["profile_name"],source,review,r["owner_answer"]!=nil))}}
 return mcpResponse(strings.Join(lines,"\n"),map[string]any{"status":"completed","taskId":publicID,"reviews":reviews,"totals":totals},meta),nil
}
