import { useState } from "react";
import {
  findLocalBusinesses,
  getLocalBusinessProfile,
  runLocalGrid,
  type LocalGridResult,
} from "./localSeoApi";

type Business = Record<string, unknown>;

export function LocalSEOPage({ projectId }: { projectId: string }) {
  const [keyword, setKeyword] = useState("");
  const [businessName, setBusinessName] = useState("");
  const [latitude, setLatitude] = useState("");
  const [longitude, setLongitude] = useState("");
  const [gridSize, setGridSize] = useState<3 | 5>(3);
  const [spacingKm, setSpacingKm] = useState(2);
  const [device, setDevice] = useState<"mobile" | "desktop">("mobile");
  const [grid, setGrid] = useState<LocalGridResult | null>(null);
  const [businesses, setBusinesses] = useState<Business[]>([]);
  const [profile, setProfile] = useState<Business | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const center = { latitude: Number(latitude), longitude: Number(longitude) };
  const coordinatesValid =
    latitude !== "" &&
    longitude !== "" &&
    Number.isFinite(center.latitude) &&
    Number.isFinite(center.longitude) &&
    center.latitude >= -90 &&
    center.latitude <= 90 &&
    center.longitude >= -180 &&
    center.longitude <= 180;
  const gridValid =
    coordinatesValid &&
    keyword.trim() !== "" &&
    businessName.trim() !== "" &&
    spacingKm >= 0.25 &&
    spacingKm <= 10;

  async function perform(action: () => Promise<void>, fallback: string) {
    setError("");
    setBusy(true);
    try {
      await action();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : fallback);
    } finally {
      setBusy(false);
    }
  }

  function searchBusinesses() {
    return perform(async () => {
      const result = await findLocalBusinesses(projectId, {
        query: businessName.trim() || keyword.trim(),
        ...center,
        radiusKm: 10,
      });
      setBusinesses(result.businesses);
    }, "Business lookup failed.");
  }

  function auditProfile() {
    return perform(async () => {
      const result = await getLocalBusinessProfile(projectId, {
        businessName: businessName.trim(),
        ...center,
        radiusKm: 10,
        languageCode: "en",
      });
      setProfile(result.profile);
    }, "Profile lookup failed.");
  }

  function runGrid() {
    return perform(async () => {
      const result = await runLocalGrid({
        projectId,
        keyword: keyword.trim(),
        target: { name: businessName.trim() },
        center,
        gridSize,
        spacingKm,
        device,
        languageCode: "en",
      });
      setGrid(result);
    }, "Local rank grid failed.");
  }

  return (
    <main className="mx-auto w-full max-w-6xl space-y-6 p-6">
      <header>
        <p className="text-sm text-muted-foreground">Local SEO</p>
        <h1 className="text-3xl font-semibold">Google Maps visibility</h1>
        <p className="mt-2 text-muted-foreground">
          Measure local rankings, audit your business listing, and discover
          nearby competitors.
        </p>
      </header>
      <section className="grid gap-4 rounded-xl border p-5 md:grid-cols-2">
        <TextField
          label="Search keyword"
          value={keyword}
          onChange={setKeyword}
          placeholder="e.g. dentist near me"
        />
        <TextField
          label="Your business name"
          value={businessName}
          onChange={setBusinessName}
          placeholder="Business name to match on Maps"
        />
        <TextField
          label="Center latitude"
          value={latitude}
          onChange={setLatitude}
          type="number"
          placeholder="e.g. 19.0760"
        />
        <TextField
          label="Center longitude"
          value={longitude}
          onChange={setLongitude}
          type="number"
          placeholder="e.g. 72.8777"
        />
        <label className="space-y-1 text-sm">
          Grid size
          <select
            className="block w-full rounded-md border px-3 py-2"
            value={gridSize}
            onChange={(event) => {
              const value = Number(event.target.value);
              if (value === 3 || value === 5) setGridSize(value);
            }}
          >
            <option value={3}>3 x 3 - 9 searches</option>
            <option value={5}>5 x 5 - 25 searches</option>
          </select>
        </label>
        <TextField
          label="Distance between points (km)"
          value={String(spacingKm)}
          onChange={(value) => setSpacingKm(Number(value))}
          type="number"
        />
        <label className="space-y-1 text-sm">
          Device
          <select
            className="block w-full rounded-md border px-3 py-2"
            value={device}
            onChange={(event) => {
              if (
                event.target.value === "mobile" ||
                event.target.value === "desktop"
              )
                setDevice(event.target.value);
            }}
          >
            <option value="mobile">Mobile</option>
            <option value="desktop">Desktop</option>
          </select>
        </label>
        <div className="flex flex-wrap items-end gap-2">
          <button
            disabled={!gridValid || busy}
            onClick={runGrid}
            className="rounded-md bg-primary px-4 py-2 text-primary-foreground disabled:opacity-50"
          >
            {busy ? "Searching..." : "Run Maps grid"}
          </button>
          <button
            disabled={!coordinatesValid || busy || !(businessName || keyword)}
            onClick={searchBusinesses}
            className="rounded-md border px-4 py-2 disabled:opacity-50"
          >
            Find nearby businesses
          </button>
          <button
            disabled={!coordinatesValid || busy || !businessName.trim()}
            onClick={auditProfile}
            className="rounded-md border px-4 py-2 disabled:opacity-50"
          >
            Audit business profile
          </button>
        </div>
        <p className="text-xs text-muted-foreground md:col-span-2">
          Each grid point sends one Maps data request (up to{" "}
          {gridSize * gridSize}); the provider may bill per request.
        </p>
        {error && (
          <p role="alert" className="text-sm text-destructive md:col-span-2">
            {error}
          </p>
        )}
      </section>
      {grid && <RankGridResults result={grid} />}
      {profile && <ProfileCard profile={profile} />}
      {businesses.length > 0 && <BusinessList businesses={businesses} />}
    </main>
  );
}

function TextField({
  label,
  value,
  onChange,
  placeholder,
  type = "text",
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  type?: "text" | "number";
}) {
  return (
    <label className="space-y-1 text-sm">
      {label}
      <input
        className="w-full rounded-md border px-3 py-2"
        type={type}
        step={type === "number" ? "any" : undefined}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
      />
    </label>
  );
}

function RankGridResults({ result }: { result: LocalGridResult }) {
  return (
    <section className="grid gap-6 rounded-xl border p-5 md:grid-cols-[1fr_280px]">
      <div>
        <h2 className="mb-4 text-xl font-semibold">
          Rank grid{" "}
          <span className="text-sm font-normal text-muted-foreground">
            North is at the top
          </span>
        </h2>
        <div
          className="mx-auto grid max-w-md gap-2"
          style={{
            gridTemplateColumns: `repeat(${Math.sqrt(result.grid.length)},minmax(0,1fr))`,
          }}
        >
          {result.grid.map((point) => (
            <div
              key={`${point.row}-${point.col}`}
              title={`${point.latitude}, ${point.longitude}`}
              className={`flex aspect-square flex-col items-center justify-center rounded-lg border ${point.error ? "bg-muted" : "bg-card"}`}
            >
              <strong className="text-2xl">
                {point.error ? "ERR" : (point.rank ?? "-")}
              </strong>
              <span className="text-xs text-muted-foreground">
                {point.error
                  ? "Failed"
                  : point.rank != null
                    ? `Rank ${point.resultsCount}`
                    : `Not found ${point.resultsCount}`}
              </span>
            </div>
          ))}
        </div>
        <p className="mt-3 text-xs text-muted-foreground">
          - means the business was not found in the returned results; ERR means
          that point failed.
        </p>
      </div>
      <div className="space-y-3">
        <h2 className="text-xl font-semibold">Grid summary</h2>
        <Metric
          label="Average rank where found"
          value={result.summary.averageRank ?? "Not found"}
        />
        <Metric
          label="Top 3 locations"
          value={`${result.summary.top3Count} / ${result.summary.pointsSearched}`}
        />
        <Metric
          label="Top 10 locations"
          value={`${result.summary.top10Count} / ${result.summary.pointsSearched}`}
        />
        <Metric label="Failed searches" value={result.summary.failedPoints} />
      </div>
    </section>
  );
}

function ProfileCard({ profile }: { profile: Business }) {
  const fields = [
    "title",
    "category",
    "address",
    "phone",
    "url",
    "is_claimed",
    "rating",
    "work_hours",
  ];
  return (
    <section className="rounded-xl border p-5">
      <h2 className="mb-3 text-xl font-semibold">Business profile audit</h2>
      <div className="grid gap-3 md:grid-cols-3">
        {fields
          .filter((field) => profile[field] != null)
          .map((field) => (
            <Metric
              key={field}
              label={field.replaceAll("_", " ")}
              value={displayValue(profile[field])}
            />
          ))}
      </div>
    </section>
  );
}

function BusinessList({ businesses }: { businesses: Business[] }) {
  return (
    <section className="rounded-xl border p-5">
      <h2 className="mb-4 text-xl font-semibold">Nearby businesses</h2>
      <ul className="divide-y">
        {businesses.map((business, index) => (
          <li
            key={
              typeof business.cid === "string"
                ? business.cid
                : typeof business.place_id === "string"
                  ? business.place_id
                  : index
            }
            className="py-3"
          >
            <strong>
              {typeof business.title === "string"
                ? business.title
                : "Unnamed business"}
            </strong>
            <span className="ml-2 text-sm text-muted-foreground">
              {business.rating != null
                ? `Rating ${displayValue(business.rating)}`
                : ""}{" "}
              {business.address ? `- ${displayValue(business.address)}` : ""}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function Metric({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-lg bg-muted/50 p-3">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="text-xl font-semibold">{value}</p>
    </div>
  );
}

function displayValue(value: unknown): string {
  if (
    typeof value === "string" ||
    typeof value === "number" ||
    typeof value === "boolean"
  )
    return String(value);
  return JSON.stringify(value) ?? "";
}
