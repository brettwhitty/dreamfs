# GitHub Docs YAML Frontmatter Reference

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `versions` | Object | 🔴 **Required** | Indicates the versions to which a page applies (e.g., `fpt`, `ghes`). Must be present for all `index.md` files. |
| `redirect_from` | Array | 🟢 **Optional** | List of URLs that should redirect to the current page. |
| `title` | String | 🔴 **Required** | Human-friendly title for the page `<title>` tag and `h1` element. |
| `shortTitle` | String | 🟢 **Optional** | Abbreviated title for breadcrumbs and navigation. Defaults to `title` if omitted. |
| `intro` | String | 🟢 **Optional** | Introduction text rendered after the title. |
| `permissions` | String | 🟢 **Optional** | Permission statement rendered after the intro. |
| `product` | String | 🟢 **Optional** | Product callout rendered after the intro and permissions. |
| `layout` | String | 🟢 **Optional** | Defines the page layout (e.g., `product-landing`). Defaults to `DefaultLayout`. |
| `children` | Array | 🔴 **Required** (Index) | Lists relative links belonging to the product/category/map topic. Default is `false`. |
| `childGroups` | Array | 🔴 **Required** (Home) | Renders children into groups on the homepage. Default is `false`. |
| `featuredLinks` | Object | 🟢 **Optional** | Renders linked articles' titles and intros on product landing pages and the homepage. |
| `showMiniToc` | Boolean | 🟢 **Optional** | Toggles the mini Table of Contents. Default is `true` on articles, `false` on map topics/index. |
| `allowTitleToDifferFromFilename` | Boolean | 🟢 **Optional** | Allows title to differ from filename without triggering test flags. Default is `false`. |
| `changelog` | Object | 🟢 **Optional** | Renders a list of items pulled from the GitHub Changelog on product landing pages. |
| `defaultPlatform` | String | 🟢 **Optional** | Overrides initial platform selection (`mac`, `windows`, `linux`). |
| `defaultTool` | String | 🟢 **Optional** | Overrides initial tool selection (e.g., `webui`, `cli`, `desktop`). |
| `learningTracks` | String | 🟢 **Optional** | References learning tracks defined in `data/learning-tracks/*.yml`. |
| `includeGuides` | Array | 🟢 **Optional** | Renders a list of articles filterable by type and topics (used with `product-guides` layout). |
| `journeyTracks` | Array | 🟢 **Optional** | Defines journeys for journey landing pages. |
| `type` | String | 🟢 **Optional** | Indicates article type (e.g., `overview`, `quick_start`, `tutorial`). |
| `topics` | Array | 🟢 **Optional** | Indicates topics covered by the article. |
| `communityRedirect` | Object | 🟢 **Optional** | Sets a custom link and name for the "Ask the GitHub community" footer link. |
| `effectiveDate` | String | 🟢 **Optional** | Sets an effective date for Terms of Service articles (Format: `YEAR-MONTH-DAY`). |
