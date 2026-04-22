-- OpenBao storage backend (not used in dev -mode but provisioned for parity)
CREATE USER openbao WITH PASSWORD 'openbao';
CREATE DATABASE openbao OWNER openbao;
GRANT ALL PRIVILEGES ON DATABASE openbao TO openbao;
