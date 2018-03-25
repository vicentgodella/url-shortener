#!/bin/bash
#This is simple bash script that is used to test access to the EC2 Parameter store.
# Install the AWS CLI
apt-get -y install python2.7 curl jq
curl -O https://bootstrap.pypa.io/get-pip.py
python2.7 get-pip.py
pip install awscli
# Getting region
EC2_AVAIL_ZONE=$(curl -s http://169.254.169.254/latest/meta-data/placement/availability-zone)
EC2_REGION=$(echo "$EC2_AVAIL_ZONE" | sed -e 's:\([0-9][0-9]*\)[a-z]*\$:\\1:')
STUDENT=$(curl -s http://169.254.169.254/latest/meta-data/iam/info | jq .InstanceProfileArn | egrep -o 'student-\w+' | cut -f2 -d'-')
# Trying to retrieve parameters from the EC2 Parameter Store
export URLSHORTENER_POSTGRESQL_HOST=$(aws ssm get-parameters --names /"$STUDENT"/prod/db/host  --with-decryption --region $EC2_REGION --output text 2>&1)
export URLSHORTENER_POSTGRESQL_USER=$(aws ssm get-parameters --names /"$STUDENT"/prod/db/user  --with-decryption --region $EC2_REGION --output text 2>&1)
export URLSHORTENER_POSTGRESQL_PASSWORD=$(aws ssm get-parameters --names /"$STUDENT"/prod/db/port  --with-decryption --region $EC2_REGION --output text 2>&1)
export URLSHORTENER_POSTGRESQL_PORT=$(aws ssm get-parameters --names /"$STUDENT"/prod/db/port  --with-decryption --region $EC2_REGION --output text 2>&1)
export URLSHORTENER_FAKELOAD=$(aws ssm get-parameters --names /"$STUDENT"/prod/fakeload  --with-decryption --region $EC2_REGION --output text 2>&1)
export URLSHORTENER_STORAGE=$(aws ssm get-parameters --names /"$STUDENT"/prod/storage  --with-decryption --region $EC2_REGION --output text 2>&1)
export URLSHORTENER_HTTP_ADDR=$(aws ssm get-parameters --names /"$STUDENT"/prod/http/addr  --with-decryption --region $EC2_REGION --output text 2>&1)

/opt/url-shortener/bin/url-shortener
