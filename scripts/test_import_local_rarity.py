"""Exercise eBird joins and filtering without downloading or using real data."""

import csv
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location('importer', Path(__file__).with_name('import-local-rarity.py'))
IMPORTER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(IMPORTER)


class ImportTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)

    def write(self, name, rows):
        path = self.root / name
        with path.open('w', newline='') as stream:
            writer = csv.DictWriter(stream, fieldnames=list(rows[0]), delimiter='\t')
            writer.writeheader()
            writer.writerows(rows)
        return path

    def sample(self, identifier, **overrides):
        return {
            'SAMPLING EVENT IDENTIFIER': identifier, 'LOCALITY ID': identifier,
            'OBSERVATION DATE': '2023-05-01', 'LATITUDE': '39.34', 'LONGITUDE': '-74.57',
            'ALL SPECIES REPORTED': '1', 'PROTOCOL CODE': 'P21', 'DURATION MINUTES': '30',
            'EFFORT DISTANCE KM': '', 'NUMBER OBSERVERS': '1', 'GROUP IDENTIFIER': '',
            **overrides,
        }

    def build(self, samples, observations):
        return IMPORTER.build_dataset(
            self.write('ebd.tsv', observations), self.write('sampling.tsv', samples),
            39.34, -74.57, 25, 'US-NJ-001', 'Atlantic County', 'Synthetic test data')

    def observation(self, identifier, name='Cardinalis cardinalis'):
        return {'SAMPLING EVENT IDENTIFIER': identifier, 'SCIENTIFIC NAME': name, 'CATEGORY': 'species'}

    def test_joins_species_deduplicates_and_excludes_unmatched_events(self):
        data = self.build(
            [self.sample('a'), self.sample('b', **{'LOCALITY ID': 'a'}), self.sample('unmatched')],
            [self.observation('a'), self.observation('a', 'Cyanocitta cristata'), self.observation('b')])
        self.assertEqual(len(data['checklists']), 1)
        self.assertEqual(data['checklists'][0]['species'], ['Cardinalis cardinalis', 'Cyanocitta cristata'])
        self.assertNotIn('OBSERVER ID', json.dumps(data))
        json.dumps(data, allow_nan=False)

    def test_filters_effort_incomplete_and_distant_samples(self):
        data = self.build([
            self.sample('good'), self.sample('incomplete', **{'ALL SPECIES REPORTED': '0'}),
            self.sample('long', **{'DURATION MINUTES': '181'}),
            self.sample('distant', **{'LATITUDE': '40.5'}),
            self.sample('missing_effort', **{'PROTOCOL CODE': 'P22'}),
        ], [self.observation(key) for key in ['good', 'incomplete', 'long', 'distant', 'missing_effort']])
        self.assertEqual([c['id'] for c in data['checklists']], ['good'])

    def test_deduplicates_shared_groups(self):
        data = self.build([
            self.sample('a', **{'GROUP IDENTIFIER': 'group'}),
            self.sample('b', **{'GROUP IDENTIFIER': 'group'}),
        ], [self.observation('a'), self.observation('b')])
        self.assertEqual(len(data['checklists']), 1)

    def test_rejects_mismatched_downloads(self):
        with self.assertRaisesRegex(ValueError, 'No matching observations'):
            self.build([self.sample('a')], [self.observation('different')])

    def test_rejects_wrong_headers(self):
        with self.assertRaisesRegex(ValueError, 'missing columns'):
            list(IMPORTER.rows(self.write('wrong.tsv', [{'wrong': 'value'}]), ['SCIENTIFIC NAME']))

    def test_unresolved_taxa_do_not_become_species_absences(self):
        uncertain = {**self.observation('unresolved'), 'CATEGORY': 'issf'}
        data = self.build([self.sample('good'), self.sample('unresolved')],
                          [self.observation('good'), uncertain])
        self.assertEqual([c['id'] for c in data['checklists']], ['good'])


if __name__ == '__main__':
    unittest.main()
