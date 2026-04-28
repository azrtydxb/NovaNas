-- Run as the postgres superuser on the NAS host:
--   sudo -u postgres psql -f /usr/share/nova-api/nova-api-init.sql
--
-- SECURITY: the password below is the default-install placeholder. Before
-- exposing the API to anything other than localhost, pick a real password
-- and run:
--   ALTER ROLE novanas WITH PASSWORD '<chosen>';
-- Then update DATABASE_URL in /etc/nova-api/env to match.

CREATE ROLE novanas WITH LOGIN PASSWORD 'novanas';
CREATE DATABASE novanas OWNER novanas;
\c novanas
GRANT ALL ON SCHEMA public TO novanas;
