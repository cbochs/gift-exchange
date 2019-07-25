import time
from random import shuffle

# OUR RELATIONSHIPS (tuples) :)
relationships = {
    'siblings': [],
    'couples': []
}

# ORDERED NAMES FOR EASE OF COPY-PASTA (OUTPUT)
parent_names_ordered = []
kid_names_ordered = []

# A HISTORY OF EACH PERSON'S PARNTER UP TO A CERTAIN DATE
# Note: This is simply a starting point. These tables will be updated
#       as each year is determined (dict of names containing list of giftees)
parents = {}

kids = {}


# CHECK FOR RELATIONSHIP STATUS
def in_relationship(p1, p2):
    for siblings in relationships['siblings']:
        if p1 in siblings and p2 in siblings:
            return True
    for couples in relationships['couples']:
        if p1 in couples and p2 in couples:
            return True


# CHECK IF PERSON 1 AND PERSON 2 CAN BE PAIRED
# a. Is there a relationship between them? (sibling or couple)
# b. Have they been chosen recently?
# c. Are they the same person?
def valid_partner(p1, p2, history):
    return not in_relationship(p1, p2) and p2 not in history[p1] and p1 is not p2


# RECURSIVELY DETERMINE A SOLUTION TO THE GIFT EXCHANGE
def find_solution(i, names, names_left, solution, history):
    if i == len(names):
        return True

    person = names[i]
    for partner in names_left:
        if valid_partner(person, partner, history):
            solution[person] = partner
            new_names_left = list(names_left)
            new_names_left.remove(partner)

            if find_solution(i+1, names, new_names_left, solution, history):
                return True
            else:
                solution[person] = None
    
    return False


# UPDATE GIFT EXCHANGE HISTORY
def update_history(history, solution, lookback_length=3):
    for person, previous_partners in history.items():
        if len(previous_partners) >= lookback_length:
            previous_partners.pop(0)
        previous_partners.append(solution[person])


# DO THE GIFT EXCHANGE
def gift_exchange(names_ordered, history, output_location, year_start=2019, year_end=2030, time_delay=0.5):
    solution = {n: None for n in names_ordered}
    
    names_random = list(names_ordered)
    shuffle(names_random)

    year = year_start
    with open(output_location, 'w') as ofile:
        while find_solution(0, names_ordered, names_random, solution, history):
            # OUTPUT SOLUTION TO CONSOLE
            print(f'YEAR: {year}')
            for name in names_ordered:
                print(f'{name:20}: {solution[name]}')
            print()
            
            # OUTPUT SOLUTION TO FILE
            ofile.write(str(year) + '\n')
            for name in names_ordered:
                ofile.write(f'{name},{solution[name]}\n')
            
            # UPDATE HISTORY
            update_history(history, solution)

            # UPDATE/CHECK YEAR
            year += 1
            if year > year_end:
                break

            # RESET SOLUTION AND SHUFFLE NAMES FOR NEXT ITERATION
            solution = {n: None for n in names_ordered}
            shuffle(names_random)

            # JUST FOR FUN, ADD A SMALL WAIT TIME
            time.sleep(time_delay)

# gift_exchange(parent_names_ordered, parents, 'gift_exchange_parents.csv', year_end=2020)
gift_exchange(kid_names_ordered, kids, 'gift_exchange_kids.csv', year_end=2020)

pass
