import { describe, expect, it } from "vitest"
import { imageDisplayWidth, swapLibraryUrls } from "./media-urls"

describe("imageDisplayWidth", () => {
  it("caps the natural width at the email content column", () => {
    expect(imageDisplayWidth(1200)).toBe(600)
    expect(imageDisplayWidth(320)).toBe(320)
  })

  it("is null when the width is unknown (PDF)", () => {
    expect(imageDisplayWidth(undefined)).toBeNull()
    expect(imageDisplayWidth(0)).toBeNull()
  })
})

describe("swapLibraryUrls", () => {
  it("replaces every occurrence of each library URL with its preview URL", () => {
    const lib = "https://app.test/t/acme/api/media/a1/content"
    const html = `<img src="${lib}"><td style="background:url(${lib})">`
    expect(swapLibraryUrls(html, [{ url: lib, preview_url: "https://s3.test/a1?sig" }])).toBe(
      `<img src="https://s3.test/a1?sig"><td style="background:url(https://s3.test/a1?sig)">`,
    )
  })

  it("leaves URLs of unknown (deleted) assets alone", () => {
    const html = `<img src="https://app.test/t/acme/api/media/gone/content">`
    expect(swapLibraryUrls(html, [])).toBe(html)
  })
})
