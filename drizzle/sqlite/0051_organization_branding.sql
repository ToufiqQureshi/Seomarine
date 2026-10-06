CREATE TABLE `organization_branding` (
	`organization_id` text PRIMARY KEY NOT NULL,
	`brand_name` text NOT NULL,
	`accent_color` text NOT NULL,
	`logo_data_url` text,
	`website_url` text,
	`updated_at` text NOT NULL,
	FOREIGN KEY (`organization_id`) REFERENCES `organization`(`id`) ON UPDATE no action ON DELETE cascade
);
