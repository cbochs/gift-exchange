import argparse
import random
import sys
import time
from collections import deque
from pathlib import Path
from typing import NamedTuple, Self, Type, TypeVar
import json
import datetime


class Participant(NamedTuple):
    group: str
    active: bool
    name: str
    first: str
    last: str


class Match(NamedTuple):
    gifter: str
    recipient: str

    def reverse(self):
        return Match(gifter=self.recipient, recipient=self.gifter)


class Relationship(NamedTuple):
    kind: str
    p1: str
    p2: str


class HistoricMatch(NamedTuple):
    year: int
    gifter: str
    recipient: str


class NamePair(tuple):
    def __hash__(self) -> int:
        return hash(repr(sorted(self)))

    def __eq__(self, other: object) -> bool:
        return isinstance(other, NamePair) and sorted(self) == sorted(other)

    def other(self, name):
        if self[0] == name:
            return self[1]
        return self[0]


class UnorderedPair(tuple):
    def __hash__(self) -> int:
        return hash(repr(sorted(self)))

    def __eq__(self, other: object) -> bool:
        return isinstance(other, UnorderedPair) and sorted(self) == sorted(other)

    def first(self):
        return self[0]

    def second(self):
        return self[1]

    def other(self, name):
        if self[0] == name:
            return self[1]
        return self[0]


class OrderedPair(tuple):
    def first(self):
        return self[0]

    def second(self):
        return self[1]

    def other(self, name):
        if self[0] == name:
            return self[1]
        return self[0]


class MatchMaker:
    MAX_ITERATIONS = 10_000

    participants: set[str] = set()
    relationships: set[UnorderedPair] = set()
    history: set[OrderedPair] = set()
    matches: set[Match] = set()
    cycle_len: int | None = None

    def __init__(self) -> None:
        pass

    def with_participants(self, participants: set[str]) -> Self:
        self.participants = participants
        return self

    def with_relationships(self, relationships: set[UnorderedPair]) -> Self:
        self.relationships = relationships
        return self

    def with_history(self, history: set[OrderedPair]) -> Self:
        self.history = history
        return self

    def with_matches(self, matches: set[Match]) -> Self:
        self.matches = matches
        return self

    def with_cycle_len(self, cyclic: int | None) -> Self:
        self.cycle_len = cyclic
        return self

    def with_seed(self, seed: int) -> Self:
        self.seed = seed
        return self

    def generate(self) -> set[Match]:
        def shuffle(names: set[str]) -> list[str]:
            return random.sample(sorted(names), len(names))

        random.seed(self.seed)

        e = [0]
        counter = 0
        while True:
            counter += 1
            solution = set(self.matches)
            found = self.search(
                e,
                solution,
                shuffle(self.participants - {m.gifter for m in self.matches}),
                shuffle(self.participants - {m.recipient for m in self.matches}),
            )

            if found:
                return solution
            if counter > MatchMaker.MAX_ITERATIONS:
                raise RuntimeError(f"No solution found. Explored {e[0]:,} solutions")

            print(f"Searching... Solutions attempted: {e[0]:,}", end="\r")

    def longest_cycles(self):
        nodes, edges = self.build_network()  # TODO

        longest = []
        alternatives = []

        stack = deque([(0, -1, [])])
        visited = [False] * len(edges)

        while stack:
            to_node, from_node, cycle = stack.pop()

            if to_node in cycle:
                if len(cycle) > len(longest):
                    longest = cycle
                    alternatives = []
                if len(cycle) == len(longest):
                    alternatives.append(cycle)
                continue

            if not visited[to_node]:
                node_edges = list(edges[to_node])
                if from_node >= 0 and from_node in node_edges:
                    node_edges.remove(from_node)

                for next_node in node_edges:
                    next_cycle = cycle + [to_node]
                    stack.append((next_node, to_node, next_cycle))

            visited[to_node] = True

        print(longest)
        print(alternatives)
        print()
        print([nodes[i] for i in longest])

    def build_network(self) -> tuple[list[str], list[list[int]]]:
        nodes = sorted(list(self.participants))

        def is_edge(i1, i2):
            return (
                # Cannot gift yourself
                i1 != i2
                # Cannot gift those who you're in a relationship with
                and UnorderedPair((nodes[i1], nodes[i2])) not in self.relationships
                # Cannot gift those who you've gifted recently
                and OrderedPair((nodes[i1], nodes[i2])) not in self.history
            )

        edges = [
            [i2 for i2 in range(len(nodes)) if is_edge(i1, i2)]
            for i1 in range(len(nodes))
        ]

        return (nodes, edges)

    def search(
        self,
        e: list[int],
        solution: set[Match],
        gifters: list[str],
        recipients: list[str],
    ) -> bool:
        if len(gifters) == 0:
            return True

        gifters_rem = list(gifters)
        gifter = gifters_rem.pop()
        for recipient in recipients:
            if gifter == recipient:
                continue

            match = Match(gifter=gifter, recipient=recipient)
            if self.is_valid(match, solution):
                solution.add(match)
                e[0] += 1

                recipients_rem = list(recipients)
                recipients_rem.remove(recipient)

                found = self.search(e, solution, gifters_rem, recipients_rem)
                if found:
                    return True

                solution.remove(match)

        return False

    # CHECK IF PERSON 1 CAN GIVE PERSON 2 A GIFT
    # 1. Are they in a relationship? (sibling or couple)
    # 2. Has person 1 had person 2 as a partner recently?
    def is_valid(self, match: Match, solution: set[Match]) -> bool:
        if UnorderedPair(match) in self.relationships:
            return False

        if OrderedPair(match) in self.history:
            return False

        if self.cycle_len:
            test_solution = set(solution)
            test_solution.add(match)
            cycle = find_cycle(test_solution, match)
            min_len = self.cycle_len if self.cycle_len > 0 else len(test_solution)
            if cycle and len(cycle) < min_len:
                return False

        return True


def find_cycle(matches: set[Match], match: Match) -> list[Match] | None:
    match_dict = {m.gifter: m.recipient for m in matches}
    match_cycle = [match]

    while True:
        current_match = match_cycle[-1]
        next_gifter = current_match.recipient
        next_recipient = match_dict.get(next_gifter)

        if next_recipient is None:
            return None

        match_cycle.append(Match(gifter=next_gifter, recipient=next_recipient))

        if next_recipient == match_cycle[0].gifter:
            break

    return match_cycle


T = TypeVar("T")


def parse_ndjson(file_path: Path, klass: Type[T]) -> list[T]:
    return [klass(**json.loads(line)) for line in file_path.read_text().splitlines()]


def print_solution(solution: set[Match], all_history: list[HistoricMatch]):
    for match in solution:
        years = filter(lambda h: match.gifter == h.gifter, all_history)
        years = filter(lambda h: match.recipient == h.recipient, years)
        years = map(lambda h: h.year, years)
        years = list(years)
        print(
            f"Gifter: {match.gifter.ljust(18)} "
            f"Recipient: {match.recipient.ljust(18)} "
            f"Years: {years}"
        )


def main(args: argparse.Namespace):
    all_participants = parse_ndjson(args.participants, Participant)
    participants = filter(lambda p: p.group == args.group, all_participants)
    participants = filter(lambda p: p.active, participants)
    participants = map(lambda p: p.name, participants)
    participants = set(participants)  # set of participant names

    all_relationships = parse_ndjson(args.relationships, Relationship)
    relationships = map(lambda m: UnorderedPair([m.p1, m.p2]), all_relationships)
    relationships = set(relationships)  # set of name pairs

    all_history = parse_ndjson(args.history, HistoricMatch)
    history = all_history
    history = filter(lambda m: m.year >= (args.year - args.lookback), all_history)
    history = filter(lambda m: m.year < args.year, history)
    history = map(lambda m: OrderedPair([m.gifter, m.recipient]), history)
    history = set(history)

    initial_matches = {
        Match(gifter=match[0], recipient=match[1])
        for match_str in args.matches
        if (match := match_str.split(":"))
    }

    extra_relationships = {
        UnorderedPair([r[0], r[1]])
        for relationship_str in args.extra_relationships
        if (r := relationship_str.split(":"))
    }

    maker = (
        MatchMaker()
        .with_participants(participants)
        .with_relationships(relationships | extra_relationships)
        .with_history(history)
        .with_matches(initial_matches)
        .with_cycle_len(args.cyclic)
        .with_seed(args.seed)
    )

    if args.network:
        maker.longest_cycles()
        return

    solution = maker.generate()

    print(f"Seed: {args.seed}")
    print(f"Year: {args.year}")
    print_solution(solution, all_history)
    print("\n")

    for participant in participants:
        unmatched = filter(lambda h: h.gifter == participant, all_history)
        unmatched = filter(lambda h: h.year >= (args.year - args.lookback), unmatched)
        unmatched = map(lambda h: h.recipient, unmatched)
        unmatched = set(participants) - set(unmatched)
        unmatched = filter(
            lambda n: UnorderedPair([participant, n]) not in relationships, unmatched
        )
        unmatched = list(unmatched)
        print(f"Gifter: {participant.ljust(18)} Not matched: {unmatched}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()

    # Group. The name of the group from the list of participants to generate an exchange
    parser.add_argument("-g", "--group", type=str, required=True)

    # Participants. An ND-JSON file containing the list of all people participating in
    # the gift exchange.
    parser.add_argument("-p", "--participants", type=Path, required=True)

    # Relationships. An ND-JSON file containing the list of all relationships (including
    # a unique "self" relationship for each participant), indicating whether two
    # participants should be considered for a match.
    parser.add_argument("-r", "--relationships", type=Path, required=True)

    # History. An ND-JSON file containing the list of all historical pairings from
    # previous gift exchanges.
    parser.add_argument("-x", "--history", type=Path, required=True)

    # Year. The year for which to generate the exchange for (default to current year).
    parser.add_argument("-y", "--year", type=int, default=datetime.date.today().year)

    # Lookback.
    parser.add_argument("-l", "--lookback", type=int, default=4)

    # Cyclic.
    parser.add_argument("-c", "--cyclic", type=int, default=None)

    #
    parser.add_argument("-m", "--matches", action="append", default=[])

    #
    parser.add_argument("-R", "--extra-relationships", action="append", default=[])

    #
    parser.add_argument("-s", "--seed", action="store", default=int(time.time()))

    # TESTING
    parser.add_argument("-n", "--network", action="store_true")

    args = parser.parse_args()

    try:
        main(args)
    except KeyboardInterrupt:
        print("\nExiting.")
        sys.exit(1)
    except RuntimeError as e:
        print(e)
        sys.exit(1)
