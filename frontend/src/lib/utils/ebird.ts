import taxonomy from '../../../../internal/classifier/data/eBird_taxonomy_codes_2021E.json';

const EBIRD_SPECIES_BASE_URL = 'https://ebird.org/species/';
const EBIRD_EXPLORE_URL = 'https://ebird.org/explore';

// Reuse BirdNET's bundled eBird taxonomy instead of guessing codes from names.
const codesByScientificName = new Map(
  Object.entries(taxonomy)
    .filter(([name]) => name.includes('_'))
    .map<[string, string]>(([name, code]) => [name.split('_')[0], code])
);

/** Link to an eBird species profile, or Explore when the taxon is not mapped. */
export function getEBirdLink(scientificName: string): { href: string; direct: boolean } {
  const code = codesByScientificName.get(scientificName.trim());
  return code
    ? { href: `${EBIRD_SPECIES_BASE_URL}${code}`, direct: true }
    : { href: EBIRD_EXPLORE_URL, direct: false };
}
