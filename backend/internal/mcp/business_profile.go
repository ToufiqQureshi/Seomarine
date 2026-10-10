package mcp

import (
 "context"
 "encoding/json"
 "fmt"
 "strings"

 "github.com/toufiqqureshi/seomarine/backend/internal/keywords"
)

func getBusinessProfileTool()*tool{return &tool{Name:"get_business_profile",Title:"Get business profile",Description:"Reads one Google Business Profile's categories, rating, address, phone, website, claimed status, hours, and photo count. Charges credits.",
 InputSchema:json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"businessName":{"type":"string","minLength":1,"maxLength":200},"cid":{"type":"string","minLength":1,"maxLength":64},"placeId":{"type":"string","minLength":1,"maxLength":256},"near":{"type":"object","properties":{"latitude":{"type":"number","minimum":-90,"maximum":90},"longitude":{"type":"number","minimum":-180,"maximum":180},"radiusKm":{"type":"number","minimum":0.2,"maximum":199}},"required":["latitude","longitude"],"additionalProperties":false},"locationCode":{"type":"integer","minimum":1},"languageCode":{"type":"string","minLength":2,"maxLength":8}},"required":["projectId"],"additionalProperties":false}`),
 OutputSchema:json.RawMessage(`{"type":"object","properties":{"profile":{"type":["object","null"]}},"required":["profile"],"additionalProperties":true}`),Annotations:map[string]any{"readOnlyHint":false,"openWorldHint":true,"destructiveHint":false},Handler:handleGetBusinessProfile}}
func handleGetBusinessProfile(ctx context.Context,raw json.RawMessage,env *callEnv)(*callResult,error){
 var a struct{ProjectID string `json:"projectId"`;BusinessName *string `json:"businessName"`;CID *string `json:"cid"`;PlaceID *string `json:"placeId"`;Near *struct{Latitude *float64 `json:"latitude"`;Longitude *float64 `json:"longitude"`;RadiusKm *float64 `json:"radiusKm"`} `json:"near"`;LocationCode *int `json:"locationCode"`;LanguageCode string `json:"languageCode"`}
 if e:=json.Unmarshal(raw,&a);e!=nil||strings.TrimSpace(a.ProjectID)==""{return nil,newAppErrorf("VALIDATION_ERROR","projectId is required")}
 count:=0;for _,v:=range []*string{a.BusinessName,a.CID,a.PlaceID}{if v!=nil{count++}};if count!=1{return nil,newAppErrorf("VALIDATION_ERROR","Provide exactly one business identifier: businessName, cid, or placeId.")}
 keyword:="";switch{case a.BusinessName!=nil&&strings.TrimSpace(*a.BusinessName)!="":if len(*a.BusinessName)>200{return nil,newAppErrorf("VALIDATION_ERROR","businessName is too long")};keyword=strings.TrimSpace(*a.BusinessName);case a.CID!=nil&&strings.TrimSpace(*a.CID)!="":if len(*a.CID)>64{return nil,newAppErrorf("VALIDATION_ERROR","cid is too long")};keyword="cid:"+strings.TrimSpace(*a.CID);case a.PlaceID!=nil&&strings.TrimSpace(*a.PlaceID)!="":if len(*a.PlaceID)>256{return nil,newAppErrorf("VALIDATION_ERROR","placeId is too long")};keyword="place_id:"+strings.TrimSpace(*a.PlaceID);default:return nil,newAppErrorf("VALIDATION_ERROR","Business identifier must not be empty.")}
 access,e:=env.h.authorizeProject(ctx,env.auth,a.ProjectID);if e!=nil{return nil,e};if env.deps.KeywordResearch==nil{return nil,newAppErrorf("SERVICE_UNAVAILABLE","Business profile lookup is not configured.")}
 provider,ok:=env.deps.KeywordResearch.Data.(keywords.BusinessInfoProvider);if !ok{return nil,newAppErrorf("SERVICE_UNAVAILABLE","Business profile lookup is not configured.")}
 code:=access.Project.LocationCode;if a.LocationCode!=nil{code=*a.LocationCode;if code<1{return nil,newAppErrorf("VALIDATION_ERROR","locationCode must be positive")}}
 language:=strings.TrimSpace(a.LanguageCode);if language==""{language=access.Project.LanguageCode};if len(language)<2||len(language)>8{return nil,newAppErrorf("VALIDATION_ERROR","languageCode must be 2 to 8 characters")}
 coordinate:="";if a.Near!=nil{if a.Near.Latitude==nil||a.Near.Longitude==nil||*a.Near.Latitude < -90||*a.Near.Latitude>90||*a.Near.Longitude < -180||*a.Near.Longitude>180{return nil,newAppErrorf("VALIDATION_ERROR","near coordinates are invalid")};radius:=10.0;if a.Near.RadiusKm!=nil{radius=*a.Near.RadiusKm};if radius<0.2||radius>199{return nil,newAppErrorf("VALIDATION_ERROR","radiusKm must be from 0.2 to 199")};coordinate=keywords.FormatBusinessDataCoordinate(*a.Near.Latitude,*a.Near.Longitude,radius)}
 profile,e:=provider.BusinessInfo(ctx,access.Auth.OrganizationID,keywords.BusinessInfoInput{Keyword:keyword,Coordinate:coordinate,LocationCode:code,LanguageCode:language});if e!=nil{env.deps.Logger.ErrorContext(ctx,"MCP business profile","project_id",access.Project.ID,"err",e);return nil,newAppErrorf("INTERNAL_ERROR","Unable to load the Google Business Profile.")}
 text:="No Google Business Profile matched that identifier. Try a cid or placeId from get_local_serp_results.";if profile!=nil{text="Google Business Profile:";for _,key:=range []string{"title","category","address","phone","url","is_claimed"}{if value,ok:=profile[key];ok{text+="\n"+key+": "+fmt.Sprint(value)}}}
 return mcpResponse(text,map[string]any{"profile":profile},metaFields{ProjectID:access.Project.ID,URL:buildDashboardURL(env.auth.BaseURL,"/p/"+access.Project.ID,nil)}),nil
}
