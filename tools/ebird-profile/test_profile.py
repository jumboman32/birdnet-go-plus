"""Import correctness checks using synthetic records only."""

import csv
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location("ebird_profile", Path(__file__).with_name("main.py"))
profile_module = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(profile_module)


class ProfileTests(unittest.TestCase):
    def write_rows(self, directory, filename, records):
        path = Path(directory, filename)
        fields = list(dict.fromkeys(key for row in records for key in row))
        with path.open("w", encoding="utf-8", newline="") as target:
            writer = csv.DictWriter(target, fields, delimiter="\t")
            writer.writeheader()
            writer.writerows(records)
        return path

    def checklist(self, event, **changes):
        row = {
            "SAMPLING EVENT IDENTIFIER": event, "GROUP IDENTIFIER": "",
            "COUNTY CODE": "US-NJ-001", "ALL SPECIES REPORTED": "1",
            "PROTOCOL CODE": "P21", "DURATION MINUTES": "30", "EFFORT DISTANCE KM": "",
            "LATITUDE": "39.5", "LONGITUDE": "-74.5", "OBSERVATION DATE": "2026-11-07",
        }
        row.update(changes)
        return row

    def observation(self, event, **changes):
        row = {
            "SAMPLING EVENT IDENTIFIER": event, "COUNTY CODE": "US-NJ-001",
            "SCIENTIFIC NAME": "Corvus corax", "CATEGORY": "species", "EXOTIC CODE": "",
            "APPROVED": "1", "OBSERVATION COUNT": "X",
        }
        row.update(changes)
        return row

    def build(self, sampling, observations):
        with tempfile.TemporaryDirectory() as directory:
            return profile_module.build_profile(
                self.write_rows(directory, "birds.txt", observations),
                self.write_rows(directory, "sampling.txt", sampling),
                "US-NJ-001", "Synthetic fixture",
            )

    def test_complete_denominator_group_dedup_subspecies_and_rejections(self):
        checklists = [
            self.checklist("S2", **{"GROUP IDENTIFIER": "G1"}),
            self.checklist("S10", **{"GROUP IDENTIFIER": "G1"}),
            self.checklist("S3"),  # A complete checklist with no raven still counts.
            self.checklist("S4", **{"ALL SPECIES REPORTED": "0"}),
            self.checklist("S5", **{"PROTOCOL CODE": "P20"}),
            self.checklist("S6", **{"OBSERVATION DATE": "2000-11-07"}),
            self.checklist("S7", **{"COUNTY CODE": "US-NJ-999"}),
            self.checklist("S8", **{"DURATION MINUTES": "301"}),
            self.checklist("S9", **{"EFFORT DISTANCE KM": "6"}),
        ]
        observations = [self.observation(f"S{number}") for number in (2, 10, 4, 5, 6, 7, 8, 9)]
        observations += [
            self.observation("S2", **{"CATEGORY": "issf", "SCIENTIFIC NAME": "Corvus corax principalis"}),
            self.observation("S3", **{"APPROVED": "0"}),
            self.observation("S3", **{"CATEGORY": "slash"}),
            self.observation("S3", **{"EXOTIC CODE": "X"}),
            self.observation("S3", **{"EXOTIC CODE": "P"}),
            self.observation("S3", **{"OBSERVATION COUNT": "0"}),
        ]
        result = self.build(checklists, observations)
        november = result["months"][10]
        self.assertEqual(2, november["checklists"])
        self.assertEqual({"Corvus corax": 1}, november["species"])
        self.assertEqual(2017, result["first_year"])
        self.assertEqual(0, result["months"][0]["checklists"])
        text = json.dumps(result)
        self.assertNotIn("SAMPLING EVENT", text)
        self.assertNotIn("OBSERVER", text)
        self.assertNotIn("G1", text)

    def test_atomic_output_replacement(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory, "profile.ebird.local.json")
            profile_module.write_profile({"version": 1}, path)
            profile_module.write_profile({"version": 2}, path)
            self.assertEqual({"version": 2}, json.loads(path.read_text()))
            self.assertEqual([path], list(Path(directory).iterdir()))

    def test_no_eligible_checklists_fails(self):
        with self.assertRaisesRegex(ValueError, "no eligible"):
            self.build([self.checklist("S1", **{"ALL SPECIES REPORTED": "0"})], [self.observation("S1")])

    def test_mismatched_downloads_fail(self):
        with self.assertRaisesRegex(ValueError, "no matching observations"):
            self.build([self.checklist("S1")], [self.observation("S2")])


if __name__ == "__main__":
    unittest.main()
