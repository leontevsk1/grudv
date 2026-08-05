ALTER TABLE nodes ADD COLUMN config JSONB;
COMMENT ON COLUMN nodes.node_type IS 'operator-facing label only; config generation driven by nodes.config JSONB for reality/web nodes. relay nodes still use legacy Go dispatch until relay gets its own declarative model.';
