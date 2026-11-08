"""Check seasonal inference and read-only microphone comparisons with synthetic data."""
import datetime as dt
from contextlib import closing
from contextlib import redirect_stderr
import io
import importlib.util
from pathlib import Path
import sqlite3
import tempfile
import unittest
from unittest.mock import patch
from atomic_output import write_atomic

SPEC = importlib.util.spec_from_file_location("bird_calendar", Path(__file__).with_name("calendar.py"))
CALENDAR = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CALENDAR)


class CalendarTests(unittest.TestCase):
    def test_watcher_retries_without_discarding_the_last_report(self):
        arguments = ["calendar.py", "--history", "history.json", "--database", "station.db",
                     "--output", "report.html", "--watch"]
        with patch("sys.argv", arguments), patch.object(CALENDAR, "ZoneInfo"), \
                patch.object(CALENDAR, "export_report", side_effect=[OSError(), None]) as export, \
                patch.object(CALENDAR.time, "sleep", side_effect=[None, KeyboardInterrupt]), \
                redirect_stderr(io.StringIO()):
            with self.assertRaises(KeyboardInterrupt):
                CALENDAR.main()
        self.assertEqual(2, export.call_count)

    def test_temporary_output_cannot_overwrite_an_input_with_same_stem(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory, "report.tmp")
            source.write_bytes(b"private original input")
            output = Path(directory, "report.html")
            write_atomic(output, b"generated report")
            self.assertEqual(b"private original input", source.read_bytes())
            self.assertEqual(b"generated report", output.read_bytes())
            self.assertEqual(2, len(list(Path(directory).iterdir())))

    def test_both_outputs_protect_the_optional_species_dictionary(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            names = root / "names.json"
            names.write_text("private original dictionary", encoding="utf-8")
            base = ["calendar.py", "--history", str(root/"history.json"), "--database", str(root/"data.db"), "--names", str(names)]
            for outputs in (["--output", str(names)], ["--output", str(root/"report.html"), "--json-output", str(names)]):
                with patch("sys.argv", base + outputs), redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
                    CALENDAR.main()
                self.assertEqual("private original dictionary", names.read_text(encoding="utf-8"))

    def test_leap_year_and_year_end_week_alignment(self):
        self.assertEqual(CALENDAR.week_of(dt.date(2024, 3, 1)), CALENDAR.week_of(dt.date(2025, 3, 1)))
        self.assertEqual(CALENDAR.week_of(dt.date(2024, 2, 28)), CALENDAR.week_of(dt.date(2024, 2, 29)))
        self.assertEqual(51, CALENDAR.week_of(dt.date(2026, 12, 31)))

    def test_winter_season_wraps_and_residents_do_not_arrive(self):
        winter = [.5] * 10 + [0] * 32 + [.5] * 10
        events = CALENDAR.seasonal_events(winter)
        self.assertEqual([42], events["arrivals"])
        self.assertEqual([10], events["departures"])
        self.assertNotIn(0, events["arrivals"])
        resident = CALENDAR.seasonal_events([.4] * 52)
        self.assertTrue(resident["resident"])
        self.assertEqual([], resident["arrivals"])
        self.assertEqual([], resident["departures"])

    def test_sparse_windows_and_current_year_do_not_create_evidence(self):
        dataset = {"checklists": []}
        for year in (2023, 2024, 2025, 2026):
            for index in range(40):
                dataset["checklists"].append({"date": f"{year}-05-01", "complete": True,
                                             "localityId": f"site{index}", "species": ["Cyanocitta cristata"]})
        rates, samples, years = CALENDAR.historical_calendar(dataset, 2026)
        week = CALENDAR.week_of(dt.date(2026, 5, 1))
        self.assertEqual(120, samples[week])
        self.assertEqual(3, years[week])
        self.assertEqual(1, rates["Cyanocitta cristata"][week])
        self.assertIsNone(rates["Cyanocitta cristata"][0])

    def test_missing_bird_checklists_are_in_denominator(self):
        dataset = {"checklists": []}
        for year in (2023, 2024, 2025):
            for index in range(40):
                dataset["checklists"].append({"date": f"{year}-05-01", "complete": True,
                                             "localityId": f"site{index}",
                                             "species": ["Cyanocitta cristata"] if index < 20 else []})
        rates, _, _ = CALENDAR.historical_calendar(dataset, 2026)
        self.assertEqual(.5, rates["Cyanocitta cristata"][CALENDAR.week_of(dt.date(2026, 5, 1))])

    def test_repeated_calls_count_one_day_and_false_positives_are_excluded(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory, "fixture.db")
            with closing(sqlite3.connect(path)) as target, target:
                target.executescript("""
                    CREATE TABLE detections(id INTEGER, label_id INTEGER, detected_at INTEGER);
                    CREATE TABLE labels(id INTEGER, scientific_name TEXT, label_type_id INTEGER);
                    CREATE TABLE label_types(id INTEGER, name TEXT);
                    CREATE TABLE detection_reviews(detection_id INTEGER, verified TEXT);
                    INSERT INTO label_types VALUES(1,'species'),(2,'noise');
                    INSERT INTO labels VALUES(1,'Cyanocitta cristata',1),(2,'Noise',2);
                    INSERT INTO detection_reviews VALUES(3,'false_positive');
                """)
                timestamp = int(dt.datetime(2026, 11, 7, 12, tzinfo=dt.timezone.utc).timestamp())
                target.executemany("INSERT INTO detections VALUES(?,?,?)", [(index, 1, timestamp) for index in (1, 2, 3)])
                target.execute("INSERT INTO detections VALUES(4,2,?)", (timestamp,))
            before = path.read_bytes()
            days, counts, station_days, dates = CALENDAR.microphone_comparison(path, 2026, dt.timezone.utc)
            week = CALENDAR.week_of(dt.date(2026, 11, 7))
            self.assertEqual(1, days["Cyanocitta cristata"][week])
            self.assertEqual(2, counts["Cyanocitta cristata"][week])
            self.assertEqual(1, station_days[week])
            self.assertEqual(["2026-11-07"], dates)
            self.assertNotIn("Noise", counts)
            self.assertEqual(before, path.read_bytes())


if __name__ == "__main__":
    unittest.main()
