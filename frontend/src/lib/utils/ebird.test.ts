import { describe, expect, it } from 'vitest';
import { getEBirdLink } from './ebird';

describe('getEBirdLink', () => {
  it.each([
    ['Cyanocitta cristata', 'blujay'],
    ['Turdus migratorius', 'amerob'],
    ['Dryobates pubescens', 'dowwoo'],
    [' Parus major ', 'gretit1'],
  ])('links %s directly to its eBird profile', (name, code) => {
    expect(getEBirdLink(name)).toEqual({
      href: `https://ebird.org/species/${code}`,
      direct: true,
    });
  });

  it.each(['Unknown species', '', 'constructor'])('keeps unmapped name %s on eBird', name => {
    expect(getEBirdLink(name)).toEqual({ href: 'https://ebird.org/explore', direct: false });
  });
});
