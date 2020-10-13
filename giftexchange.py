import argparse
import csv
from collections import OrderedDict
from pprint import pprint
from random import shuffle
from time import sleep


def read_in_relationships(relationship_file):
    relationships = []
    with open(relationship_file, 'r') as f:
        reader = csv.reader(f, delimiter=',')
        relationships = [
            tuple(map(lambda x: x.strip() , row))
            for row in reader]
    return relationships


def read_in_history(history_file):
    history = {}
    with open(history_file, 'r') as f:
        reader = csv.reader(f, delimiter=',')
        headers = list(map(lambda x: x.strip(), next(reader)))
        history = {name: [] for name in headers}
        for row in reader:
            for i, name in enumerate(headers):
                if row[i].strip() == 'N/A':
                    row[i] = None
                else:
                    row[i] = row[i].strip()
                history[name].append(row[i])

    return headers, history


# GENERATE GIFT EXCHANGE SOLUTION: ENTRY POINT
def generate_solution(names, relationships, history):
    counter    = 0
    gifters    = list(names)
    recipients = list(names)

    solution = OrderedDict()
    for name in names:
        solution[name] = None

    while True:
        counter += 1
        shuffle(recipients)
        found = _generate_solution(
            solution, gifters, recipients,
            relationships, history)
        if found:
            break
        if counter > 1000:
            raise RuntimeError('Could not determine solution')

    return solution


# GENERATE GIFT EXCHANGE SOLUTION: RECURSIVE STEP
def _generate_solution(solution, gifters_remaining, recipients_remaining,
                       relationships, history):
    if len(gifters_remaining) == 0:
        return True

    gifters_remaining = list(gifters_remaining)
    gifter = gifters_remaining.pop()
    for recipient in recipients_remaining:
        is_valid_recipient = valid_recipient(
            gifter, recipient, relationships, history, solution)

        if is_valid_recipient:
            solution[gifter] = recipient
            recipients_remaining = list(recipients_remaining)
            recipients_remaining.remove(recipient)

            solution_found = _generate_solution(
                solution, gifters_remaining, recipients_remaining,
                relationships, history)

            if solution_found:
                return True
            else:
                solution[gifter] = None

    return False


# CHECK IF PERSON 1 CAN GIVE PERSON 2 A GIFT
# a. Are they in a relationship? (sibling or couple)
# b. Has person 1 had person 2 as a partner recently?
# c. Are they the same person?
# d. Are they gifting each other?
def valid_recipient(gifter, recipient, relationships, history, solution):
    return not in_relationship(gifter, recipient, relationships) \
           and not in_history(gifter, recipient, history) \
           and not solution[recipient] == gifter \
           and gifter is not recipient


def in_relationship(p1, p2, relationships):
    for r in relationships:
        if p1 in r and p2 in r:
            return True
    return False


def in_history(gifter, recipient, history, max_lookback=3):
    return recipient in history[gifter][-max_lookback:]


def print_solution(solution, year):
    pprint(solution) # okay, could print better than this...


def save_solution(solution, year, history_file):
    with open(history_file, 'a') as f:
        f.write("\n")
        writer = csv.writer(f, delimiter=',')
        writer.writerow([year] + list(solution.values()))


parser = argparse.ArgumentParser()
parser.add_argument('-r', '--relationships', action='store', required=True)
parser.add_argument('-p', '--history',       action='store', required=True)
parser.add_argument('-n', '--years',         action='store', type=int, default=1)
parser.add_argument('-a', '--append',        action='store_true')
args = parser.parse_args()

relationships = read_in_relationships(args.relationships)

headers, history = read_in_history(args.history)

names = list(headers)
names.remove('Year')

year = int(history['Year'][-1]) + 1

for _ in range(args.years):
    solution = generate_solution(names, relationships, history)
    print_solution(solution, year)
    if args.append:
        save_solution(solution, year, args.history)

print('Done.')
