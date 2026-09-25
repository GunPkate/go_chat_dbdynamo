#!/bin/sh
# Seeds the Products table with a small sample hotel catalog (room
# service, minibar, and shop items) so the AI features have something
# real to search/recommend/reason over. Safe to re-run — it just
# overwrites the same fixed IDs each time.
set -e

ENDPOINT="http://dynamodb-local:8000"
TABLE="Products"

put() {
  aws dynamodb put-item --endpoint-url "$ENDPOINT" --table-name "$TABLE" --item "$1" >/dev/null
}

echo "seeding $TABLE..."

put '{"id":{"S":"p-club-sandwich"},"name":{"S":"Club Sandwich"},"description":{"S":"Triple-decker with turkey, bacon, lettuce and tomato, served with fries."},"category":{"S":"Room Service"},"price":{"N":"16.5"},"stock":{"N":"12"},"tags":{"L":[{"S":"lunch"},{"S":"quick"},{"S":"contains meat"}]}}'
put '{"id":{"S":"p-caesar-salad"},"name":{"S":"Caesar Salad"},"description":{"S":"Romaine, parmesan, croutons, grilled chicken optional."},"category":{"S":"Room Service"},"price":{"N":"13.0"},"stock":{"N":"9"},"tags":{"L":[{"S":"light"},{"S":"lunch"},{"S":"vegetarian option"}]}}'
put '{"id":{"S":"p-margherita-pizza"},"name":{"S":"Margherita Pizza"},"description":{"S":"Wood-fired, San Marzano tomato, fresh mozzarella, basil."},"category":{"S":"Room Service"},"price":{"N":"18.0"},"stock":{"N":"0"},"tags":{"L":[{"S":"dinner"},{"S":"vegetarian"},{"S":"shareable"}]}}'
put '{"id":{"S":"p-spicy-noodles"},"name":{"S":"Spicy Szechuan Noodles"},"description":{"S":"Hand-pulled noodles, chili oil, peanuts, scallion — served fast."},"category":{"S":"Room Service"},"price":{"N":"15.0"},"stock":{"N":"7"},"tags":{"L":[{"S":"spicy"},{"S":"quick"},{"S":"vegetarian"}]}}'
put '{"id":{"S":"p-bottled-water"},"name":{"S":"Bottled Water"},"description":{"S":"500ml still spring water."},"category":{"S":"Minibar"},"price":{"N":"4.0"},"stock":{"N":"40"},"tags":{"L":[{"S":"beverage"},{"S":"non-alcoholic"}]}}'
put '{"id":{"S":"p-local-ipa"},"name":{"S":"Local IPA Beer"},"description":{"S":"330ml bottle from a regional craft brewery."},"category":{"S":"Minibar"},"price":{"N":"8.0"},"stock":{"N":"3"},"tags":{"L":[{"S":"beverage"},{"S":"alcoholic"}]}}'
put '{"id":{"S":"p-house-red"},"name":{"S":"House Red Wine (Glass)"},"description":{"S":"Regional cabernet blend, 150ml pour."},"category":{"S":"Minibar"},"price":{"N":"11.0"},"stock":{"N":"0"},"tags":{"L":[{"S":"beverage"},{"S":"alcoholic"},{"S":"celebration"}]}}'
put '{"id":{"S":"p-sparkling-wine"},"name":{"S":"Sparkling Wine (Half Bottle)"},"description":{"S":"Dry sparkling wine, 375ml, chilled on delivery."},"category":{"S":"Minibar"},"price":{"N":"22.0"},"stock":{"N":"5"},"tags":{"L":[{"S":"beverage"},{"S":"alcoholic"},{"S":"celebration"},{"S":"anniversary"}]}}'
put '{"id":{"S":"p-extra-towels"},"name":{"S":"Extra Towel Set"},"description":{"S":"Two bath towels, two hand towels."},"category":{"S":"Amenity"},"price":{"N":"0.0"},"stock":{"N":"25"},"tags":{"L":[{"S":"amenity"},{"S":"free"}]}}'
put '{"id":{"S":"p-toothbrush-kit"},"name":{"S":"Toothbrush Kit"},"description":{"S":"Travel toothbrush and small toothpaste."},"category":{"S":"Amenity"},"price":{"N":"0.0"},"stock":{"N":"2"},"tags":{"L":[{"S":"amenity"},{"S":"free"},{"S":"toiletries"}]}}'
put '{"id":{"S":"p-yoga-mat"},"name":{"S":"Yoga Mat Rental"},"description":{"S":"In-room yoga mat, returned at checkout."},"category":{"S":"Shop"},"price":{"N":"5.0"},"stock":{"N":"6"},"tags":{"L":[{"S":"wellness"},{"S":"fitness"}]}}'
put '{"id":{"S":"p-bathrobe"},"name":{"S":"Plush Bathrobe (Purchase)"},"description":{"S":"Hotel-branded cotton bathrobe, yours to keep."},"category":{"S":"Shop"},"price":{"N":"45.0"},"stock":{"N":"1"},"tags":{"L":[{"S":"souvenir"},{"S":"comfort"}]}}'

echo "seed complete"
