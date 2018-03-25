#!/bin/bash
#This is simple bash script that is used to test access to the EC2 Parameter store.
# Install the AWS CLI
apt-get -y install python2.7 curl jq
curl -o /tmp/get-pip.py https://bootstrap.pypa.io/get-pip.py
python2.7 /tmp/get-pip.py
pip install awscli
# Getting region
EC2_AVAIL_ZONE=$(curl -s http://169.254.169.254/latest/meta-data/placement/availability-zone)
EC2_REGION=$(curl --silent http://169.254.169.254/latest/dynamic/instance-identity/document | jq -r .region)
STUDENT=$(curl -s http://169.254.169.254/latest/meta-data/iam/info | jq .InstanceProfileArn | egrep -o 'student-\w+' | cut -f2 -d'-')

# Trying to retrieve parameters from the EC2 Parameter Store
export URLSHORTENER_POSTGRESQL_HOST=$(aws ssm get-parameters --names /"$STUDENT"/prod/db/host  --with-decryption --region $EC2_REGION --output text 2>&1)
export URLSHORTENER_POSTGRESQL_USER=$(aws ssm get-parameters --names /"$STUDENT"/prod/db/user  --with-decryption --region $EC2_REGION --output text 2>&1)
export URLSHORTENER_POSTGRESQL_PASSWORD=$(aws ssm get-parameters --names /"$STUDENT"/prod/db/port  --with-decryption --region $EC2_REGION --output text 2>&1)
export URLSHORTENER_POSTGRESQL_PORT=$(aws ssm get-parameters --names /"$STUDENT"/prod/db/port  --with-decryption --region $EC2_REGION --output text 2>&1)
export URLSHORTENER_FAKELOAD=$(aws ssm get-parameters --names /"$STUDENT"/prod/fakeload  --with-decryption --region $EC2_REGION --output text 2>&1)
export URLSHORTENER_STORAGE=$(aws ssm get-parameters --names /"$STUDENT"/prod/storage  --with-decryption --region $EC2_REGION --output text 2>&1)
export URLSHORTENER_HTTP_ADDR=$(aws ssm get-parameters --names /"$STUDENT"/prod/http/addr  --with-decryption --region $EC2_REGION --output text 2>&1)
if [[ -z "${URLSHORTENER_HTTP_ADDR// }" ]];then
    unset URLSHORTENER_HTTP_ADDR
elif [[ -z "${URLSHORTENER_STORAGE// }" ]];then
    unset URLSHORTENER_STORAGE
elif [[ -z "${URLSHORTENER_FAKELOAD// }" ]];then
    unset URLSHORTENER_FAKELOAD
elif [[ -z "${URLSHORTENER_POSTGRESQL_PORT// }" ]];then
    unset URLSHORTENER_POSTGRESQL_PORT
elif [[ -z "${URLSHORTENER_POSTGRESQL_PASSWORD// }" ]];then
    unset URLSHORTENER_POSTGRESQL_PASSWORD
elif [[ -z "${URLSHORTENER_POSTGRESQL_USER// }" ]];then
    unset URLSHORTENER_POSTGRESQL_USER
elif [[ -z "${URLSHORTENER_POSTGRESQL_HOST// }" ]];then
    unset URLSHORTENER_POSTGRESQL_HOST
fi


/opt/url-shortener/bin/url-shortener
