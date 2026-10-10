package mcp

import (
 "context"
 "encoding/json"
 "fmt"
 "strings"
 "time"

 "github.com/toufiqqureshi/seomarine/backend/internal/keywords"
)

var businessUpdateFields=[]string{"rank_absolute","author","post_date","timestamp","post_text","snippet","url","links"}
func getBusinessUpdatesTool()*tool{return &tool{Name:"get_business_updates",Title:"Get business updates",Description:"Collects updates, offers, and events published on a Google Business Profile. Returns a resumable task ID while collection runs.",
 InputSchema:json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"businessName":{"type":"string","minLength":1,"maxLength":200},"cid":{"type":"string","minLength":1,"maxLength":64},"placeId":{"type":"string","minLength":1,"maxLength":256},"near":{"type":"object","properties":{"latitude":{"type":"number","minimum":-90,"maximum":90},"longitude":{"type":"number","minimum":-180,"maximum":180},"radiusKm":{"type":"number","minimum":0.2,"maximum":199}},"required":["latitude","longitude"],"additionalProperties":false},"locationCode":{"type":"integer","minimum":1},"languageCode":{"type":"string","minLength":2,"maxLength":8},"depth":{"type":"integer","minimum":10,"maximum":100},"taskId":{"type":"string","minLength":1,"maxLength":128}},"required":["projectId"],"additionalProperties":false}`),
 OutputSchema:json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","enum":["completed","processing"]},"taskId":{"type":"string"},"updates":{"type":"array","items":{"type":"object"}}},"required":["status","taskId"],"additionalProperties":true}`),Annotations:map[string]any{"readOnlyHint":false,"openWorldHint":true,"destructiveHint":false},Handler:handleGetBusinessUpdates}}
func handleGetBusinessUpdates(ctx context.Context,raw json.RawMessage,env *callEnv)(*callResult,error){
 var a struct{ProjectID string `json:"projectId"`;BusinessName *string `json:"businessName"`;CID *string `json:"cid"`;PlaceID *string `json:"placeId"`;Near *struct{Latitude *float64 `json:"latitude"`;Longitude *float64 `json:"longitude"`;RadiusKm *float64 `json:"radiusKm"`} `json:"near"`;LocationCode *int `json:"locationCode"`;LanguageCode string `json:"languageCode"`;Depth int `json:"depth"`;TaskID string `json:"taskId"`}
 if e:=json.Unmarshal(raw,&a);e!=nil||strings.TrimSpace(a.ProjectID)==""{return nil,newAppErrorf("VALIDATION_ERROR","projectId is required")}
 if a.TaskID!=""&&(len(a.TaskID)>128||strings.Contains(a.TaskID,":")){return nil,newAppErrorf("VALIDATION_ERROR","Pass the bare taskId returned by get_business_updates.")}
 if a.TaskID==""{count:=0;for _,v:=range []*string{a.BusinessName,a.CID,a.PlaceID}{if v!=nil{count++}};if count!=1{return nil,newAppErrorf("VALIDATION_ERROR","Provide exactly one business identifier: businessName, cid, or placeId.")}}
 if a.Depth!=0&&(a.Depth<10||a.Depth>100){return nil,newAppErrorf("VALIDATION_ERROR","depth must be from 10 to 100")}
 if a.Near!=nil&&(a.Near.Latitude==nil||a.Near.Longitude==nil||*a.Near.Latitude < -90||*a.Near.Latitude>90||*a.Near.Longitude < -180||*a.Near.Longitude>180){return nil,newAppErrorf("VALIDATION_ERROR","near coordinates are invalid")}
 if a.Near!=nil&&a.Near.RadiusKm!=nil&&(*a.Near.RadiusKm<0.2||*a.Near.RadiusKm>199){return nil,newAppErrorf("VALIDATION_ERROR","radiusKm must be from 0.2 to 199")}
 if a.LocationCode!=nil&&*a.LocationCode<1{return nil,newAppErrorf("VALIDATION_ERROR","locationCode must be positive")}
 if a.LanguageCode!=""&&(len(strings.TrimSpace(a.LanguageCode))<2||len(strings.TrimSpace(a.LanguageCode))>8){return nil,newAppErrorf("VALIDATION_ERROR","languageCode must be 2 to 8 characters")}
 access,e:=env.h.authorizeProject(ctx,env.auth,a.ProjectID);if e!=nil{return nil,e};if env.deps.KeywordResearch==nil{return nil,newAppErrorf("SERVICE_UNAVAILABLE","Business updates are not configured.")}
 provider,ok:=env.deps.KeywordResearch.Data.(keywords.BusinessUpdatesTaskProvider);if !ok{return nil,newAppErrorf("SERVICE_UNAVAILABLE","Business updates are not configured.")}
 taskID:=a.TaskID
 if taskID==""{
  keyword:="";if a.BusinessName!=nil{if len(*a.BusinessName)>200||strings.TrimSpace(*a.BusinessName)==""{return nil,newAppErrorf("VALIDATION_ERROR","businessName is invalid")};keyword=strings.TrimSpace(*a.BusinessName)}else if a.CID!=nil{if len(*a.CID)>64||strings.TrimSpace(*a.CID)==""{return nil,newAppErrorf("VALIDATION_ERROR","cid is invalid")};keyword="cid:"+strings.TrimSpace(*a.CID)}else{if len(*a.PlaceID)>256||strings.TrimSpace(*a.PlaceID)==""{return nil,newAppErrorf("VALIDATION_ERROR","placeId is invalid")};keyword="place_id:"+strings.TrimSpace(*a.PlaceID)}
  code:=access.Project.LocationCode;if a.LocationCode!=nil{code=*a.LocationCode};lang:=strings.TrimSpace(a.LanguageCode);if lang==""{lang=access.Project.LanguageCode}
  coord:="";if a.Near!=nil{radius:=10.0;if a.Near.RadiusKm!=nil{radius=*a.Near.RadiusKm};coord=keywords.FormatBusinessDataCoordinate(*a.Near.Latitude,*a.Near.Longitude,radius)}
  depth:=a.Depth;if depth==0{depth=10};id,err:=provider.StartBusinessUpdatesTask(ctx,access.Auth.OrganizationID,keywords.BusinessUpdatesTaskInput{Keyword:keyword,Coordinate:coord,LocationCode:code,LanguageCode:lang,Depth:depth});if err!=nil{env.deps.Logger.ErrorContext(ctx,"MCP post business updates","project_id",access.Project.ID,"err",err);return nil,newAppErrorf("INTERNAL_ERROR","Unable to start Google Business update collection.")};taskID=id
 }
 var result map[string]any;pending:=false
 for attempt:=0;attempt<6;attempt++{if attempt>0{timer:=time.NewTimer(4*time.Second);select{case <-ctx.Done():timer.Stop();pending=true;attempt=6;case <-timer.C:}};if pending{break};pending,result,e=provider.PollBusinessUpdatesTask(ctx,taskID);if e!=nil{env.deps.Logger.ErrorContext(ctx,"MCP collect business updates","project_id",access.Project.ID,"err",e);return nil,newAppErrorf("INTERNAL_ERROR",e.Error()+` The queued task is still collectable — call again with taskId "`+taskID+`" at no extra cost.`)};if !pending{break}}
 meta:=metaFields{ProjectID:access.Project.ID,URL:buildDashboardURL(env.auth.BaseURL,"/p/"+access.Project.ID,nil)}
 if pending{return mcpResponse(fmt.Sprintf("Post collection is still running. Call get_business_updates again with taskId %q in 30-60 seconds — resuming charges no extra credits.",taskID),map[string]any{"status":"processing","taskId":taskID},meta),nil}
 rawRows,_:=result["items"].([]any);updates:=make([]map[string]any,0,len(rawRows));for _,item:=range rawRows{if row,ok:=item.(map[string]any);ok{updates=append(updates,pickMCPFields(row,businessUpdateFields))}}
 lines:=[]string{fmt.Sprintf("Collected %d Google Business updates.",len(updates))};for _,r:=range updates{lines=append(lines,fmt.Sprintf("%v | %v | %v | %v",r["rank_absolute"],r["post_date"],r["author"],r["post_text"]))}
 return mcpResponse(strings.Join(lines,"\n"),map[string]any{"status":"completed","taskId":taskID,"updates":updates},meta),nil
}
