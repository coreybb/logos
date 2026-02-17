CREATE TYPE edition_delivery_interval AS ENUM
  ('hourly', 'daily', 'weekly', 'monthly')
;


CREATE TYPE edition_format AS ENUM('epub', 'mobi', 'pdf');


CREATE TYPE delivery_destination_type AS ENUM('api', 'email', 'webhook');


CREATE TYPE reading_source_type AS ENUM('api', 'email', 'rss');


CREATE TYPE delivery_status AS ENUM
  ('delivered', 'failed', 'pending', 'processing')
;


CREATE TABLE users(
  id uuid NOT NULL,
  created_at timestamp NOT NULL,
  email varchar(255) NOT NULL,
  email_token varchar(32) NOT NULL,
  CONSTRAINT users_pkey PRIMARY KEY(id)
);


CREATE TABLE delivery_destinations_base(
  id uuid NOT NULL,
  user_id uuid NOT NULL,
  created_at timestamp NOT NULL,
  is_default bool NOT NULL,
  "name" text NOT NULL,
  "type" delivery_destination_type NOT NULL,
  CONSTRAINT delivery_destinations_base_pkey PRIMARY KEY(id)
);


CREATE TABLE allowed_senders(
  id uuid NOT NULL,
  user_id uuid NOT NULL,
  created_at timestamp NOT NULL,
  email_pattern varchar(255) NOT NULL,
  "name" text NOT NULL,
  CONSTRAINT allowed_senders_pkey PRIMARY KEY(id)
);


CREATE TABLE readings(
  id uuid NOT NULL,
  reading_source_id uuid NOT NULL,
  author text,
  created_at timestamp NOT NULL,
  content_hash varchar(64) NOT NULL,
  excerpt text NOT NULL,
  published_at timestamp with time zone,
  storage_path varchar(255) NOT NULL,
  title text NOT NULL,
  CONSTRAINT readings_pkey PRIMARY KEY(id)
);


CREATE TABLE user_readings(
  reading_id uuid NOT NULL,
  user_id uuid NOT NULL,
  created_at timestamp with time zone NOT NULL,
  received_at timestamp with time zone NOT NULL,
  CONSTRAINT user_readings_pkey PRIMARY KEY(user_id, reading_id)
);


CREATE TABLE editions(
  id uuid NOT NULL,
  user_id uuid NOT NULL,
  edition_template_id uuid NOT NULL,
  "name" varchar(100) NOT NULL,
  CONSTRAINT editions_pkey PRIMARY KEY(id)
);


CREATE TABLE edition_templates(
  id uuid NOT NULL,
  user_id uuid NOT NULL,
  created_at timestamp NOT NULL,
  delivery_interval edition_delivery_interval NOT NULL,
  delivery_time time NOT NULL,
  description text,
  format edition_format NOT NULL,
  is_recurring boolean NOT NULL,
  "name" text NOT NULL,
  CONSTRAINT edition_templates_pkey PRIMARY KEY(id)
);


CREATE TABLE reading_sources(
  id uuid NOT NULL,
  created_at timestamp NOT NULL,
  "name" text NOT NULL,
  "type" reading_source_type NOT NULL,
  identifier varchar NOT NULL,
  CONSTRAINT reading_sources_pkey PRIMARY KEY(id)
);


CREATE TABLE user_reading_sources(
  reading_source_id uuid NOT NULL,
  user_id uuid NOT NULL,
  created_at timestamp NOT NULL,
  CONSTRAINT user_reading_sources_pkey PRIMARY KEY(reading_source_id, user_id)
);


CREATE TABLE edition_readings(
  edition_id uuid NOT NULL,
  reading_id uuid NOT NULL,
  created_at timestamp NOT NULL,
  CONSTRAINT edition_readings_pkey PRIMARY KEY(edition_id, reading_id)
);


CREATE TABLE deliveries(
  id uuid NOT NULL,
  edition_id uuid NOT NULL,
  delivery_destination_id uuid NOT NULL,
  created_at timestamp NOT NULL,
  completed_at timestamp,
  edition_format edition_format NOT NULL,
  file_path text NOT NULL,
  file_size integer NOT NULL,
  started_at timestamp,
  status delivery_status NOT NULL,
  CONSTRAINT deliveries_pkey PRIMARY KEY(id)
);


CREATE TABLE delivery_attempts(
  id uuid NOT NULL,
  delivery_id uuid NOT NULL,
  created_at timestamp NOT NULL,
  error_message text,
  status delivery_status NOT NULL,
  CONSTRAINT delivery_attempts_pkey PRIMARY KEY(id)
);


CREATE TABLE email_destinations(
id uuid NOT NULL, email_address varchar(255) NOT NULL,
  CONSTRAINT email_destinations_pkey PRIMARY KEY(id)
);


ALTER TABLE delivery_destinations_base
  ADD CONSTRAINT delivery_destinations_base_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users (id)
;


ALTER TABLE allowed_senders
  ADD CONSTRAINT allowed_senders_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users (id)
;


ALTER TABLE user_readings
  ADD CONSTRAINT user_readings_reading_id_fkey
    FOREIGN KEY (reading_id) REFERENCES readings (id) ON DELETE Cascade
;


ALTER TABLE user_readings
  ADD CONSTRAINT user_readings_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE Cascade
;


ALTER TABLE editions
  ADD CONSTRAINT editions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id)
;


ALTER TABLE edition_templates
  ADD CONSTRAINT edition_templates_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users (id)
;


ALTER TABLE readings
  ADD CONSTRAINT readings_reading_source_id_fkey
    FOREIGN KEY (reading_source_id) REFERENCES reading_sources (id)
;


ALTER TABLE user_reading_sources
  ADD CONSTRAINT user_reading_sources_reading_source_id_fkey
    FOREIGN KEY (reading_source_id) REFERENCES reading_sources (id)
;


ALTER TABLE user_reading_sources
  ADD CONSTRAINT user_reading_sources_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users (id)
;


ALTER TABLE edition_readings
  ADD CONSTRAINT edition_readings_edition_id_fkey
    FOREIGN KEY (edition_id) REFERENCES editions (id)
;


ALTER TABLE edition_readings
  ADD CONSTRAINT edition_readings_reading_id_fkey
    FOREIGN KEY (reading_id) REFERENCES readings (id)
;


ALTER TABLE deliveries
  ADD CONSTRAINT deliveries_delivery_destination_id_fkey
    FOREIGN KEY (delivery_destination_id) REFERENCES delivery_destinations_base (id)
;


ALTER TABLE delivery_attempts
  ADD CONSTRAINT delivery_attempts_delivery_id_fkey
    FOREIGN KEY (delivery_id) REFERENCES deliveries (id)
;


ALTER TABLE deliveries
  ADD CONSTRAINT deliveries_edition_id_fkey
    FOREIGN KEY (edition_id) REFERENCES editions (id)
;


ALTER TABLE editions
  ADD CONSTRAINT editions_edition_template_id_fkey
    FOREIGN KEY (edition_template_id) REFERENCES edition_templates (id)
;


ALTER TABLE email_destinations
  ADD CONSTRAINT email_destinations_id_fkey
    FOREIGN KEY (id) REFERENCES delivery_destinations_base (id)
;
