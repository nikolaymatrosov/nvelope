-- Track B: the media library is private. The public URL is no longer a
-- property of an asset (content stores an app-origin library URL; public
-- copies are made at send time), so public_url goes. width/height feed the
-- email renderer's width attribute; published_at marks that the asset's
-- public copy exists.
ALTER TABLE media_assets
    DROP COLUMN public_url,
    ADD COLUMN width        int CHECK (width > 0),
    ADD COLUMN height       int CHECK (height > 0),
    ADD COLUMN published_at timestamptz;
